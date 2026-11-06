-- local_tz(): time zone of the installation; "days" in the COP and calibration views are local days.
CREATE OR REPLACE FUNCTION local_tz() RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT 'Europe/Amsterdam'::text $$;

-- water_wh_per_litre_kelvin(): specific heat of water per litre, for P[W] = L/h * K * this.
-- 1.163 Wh/(L*K) = water at ~20 C (cp 4.18 kJ/kg/K, 0.998 kg/L); at 30-50 C the real value is ~1 %
-- lower. Absorbed by the daily calibration against the counter (ADR-0003).
CREATE OR REPLACE FUNCTION water_wh_per_litre_kelvin() RETURNS float8
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT 1.163::float8 $$;

-- max_counter_gap(device): longest allowed distance between a minute's last counter sample and the
-- previous minute's last sample for a delta to count. Normal spacing is ~60 s (Electricity Meter
-- 10 s cadence, Heat Meter 60 s poll; jitter shifts it by a second or two). A genuinely missed
-- poll gives ~120 s, which must be excluded: otherwise a 2-minute heat delta would be paired with
-- a 1-minute elec delta and overstate COP. 90 s separates the two cases.
-- TUNE HERE: slowing the Heat Meter poll (plan risk: 120-300 s) requires raising the heat_meter
-- limit to poll interval + jitter, e.g. 150 s for a 120 s poll.
CREATE OR REPLACE FUNCTION max_counter_gap(device text) RETURNS interval
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE device
        WHEN 'heat_meter'        THEN INTERVAL '90 seconds'
        WHEN 'electricity_meter' THEN INTERVAL '90 seconds'
    END
$$;

-- heat_intervals_1m: Heat Meter interval energies (ADR-0003), one row per Heat Meter sample that
-- has a previous sample; the interval is (previous sample, this sample], attributed to the minute
-- (bucket) of the later sample. Internal building block of energy_1m and heat_calibration_daily.
--   flow_dt_kwh  thermal energy from flow rate x Delta T: trapezoid of the thermal power at the
--                two samples times the real time between them.
--                P[W] = flow_rate[L/h] * (flow_temp - return_temp)[K] * water_wh_per_litre_kelvin()
--                Negative Delta T (Defrost: heat taken from the water) counts as 0 W: the Heat
--                Meter counter does not decrease (ASSUMPTION). Trapezoid rather than the previous
--                sample's power so that a power step is split over both neighbouring minutes
--                instead of lagging one minute.
--   counter_kwh  Heat Meter energy counter delta over the same interval (1 kWh resolution).
--   seconds      interval length.
-- Both are NULL (excluded) when the previous sample is more than max_counter_gap('heat_meter')
-- older (missed poll); flow_dt_kwh also when a sample lacks flow rate or a temperature, and
-- counter_kwh on a negative delta (meter reset).
CREATE OR REPLACE VIEW heat_intervals_1m AS
WITH samples AS (
    SELECT bucket,
           max(last_time)  FILTER (WHERE metric = 'energy_kwh')   AS t,
           max(last_value) FILTER (WHERE metric = 'energy_kwh')   AS counter,
           CASE WHEN count(*) FILTER (WHERE metric IN ('flow_rate', 'flow_temp', 'return_temp')) = 3
                THEN greatest(max(last_value) FILTER (WHERE metric = 'flow_rate')
                              * (max(last_value) FILTER (WHERE metric = 'flow_temp')
                                 - max(last_value) FILTER (WHERE metric = 'return_temp'))
                              * water_wh_per_litre_kelvin(), 0)
           END                                                    AS power_w
    FROM readings_1m
    WHERE device = 'heat_meter' AND metric IN ('energy_kwh', 'flow_rate', 'flow_temp', 'return_temp')
    GROUP BY bucket
), paired AS (
    SELECT bucket, t, counter, power_w,
           lag(t)       OVER w AS prev_t,
           lag(counter) OVER w AS prev_counter,
           lag(power_w) OVER w AS prev_power_w
    FROM samples
    WHERE t IS NOT NULL
    WINDOW w AS (ORDER BY bucket)
)
SELECT
    bucket,
    CASE WHEN t - prev_t <= max_counter_gap('heat_meter')
         THEN (power_w + prev_power_w) / 2 * extract(epoch FROM t - prev_t) / 3600 / 1000
    END::float8 AS flow_dt_kwh,
    CASE WHEN t - prev_t <= max_counter_gap('heat_meter') AND counter >= prev_counter
         THEN counter - prev_counter
    END::float8 AS counter_kwh,
    extract(epoch FROM t - prev_t)::float8 AS seconds
FROM paired
WHERE prev_t IS NOT NULL;

-- heat_calibration_daily: per Europe/Amsterdam day, how the flow x Delta T heat compares with the
-- Heat Meter counter (ADR-0003). Only intervals where BOTH are known count, so gaps and meter
-- resets drop out of both sides equally.
--   counter_kwh, flow_dt_kwh   the two sums; factor = counter_kwh / flow_dt_kwh (NULL if 0).
--   minutes                    covered time in minutes.
--   valid                      the factor is used by energy_1m: day (nearly) complete (covered
--                              time >= 90 % of the local day, 23/25 h DST days handled),
--                              counter_kwh >= 5 and flow_dt_kwh >= 5 (counter has 1 kWh
--                              resolution: below ~5 kWh its quantisation error is too big), and
--                              0.8 <= factor <= 1.25 (outside: sensor/meter problem, not
--                              calibration; 1.0 is used instead).
-- The current day is normally not complete, so not valid; see energy_1m for the fallback.
CREATE OR REPLACE VIEW heat_calibration_daily AS
SELECT
    day,
    counter_kwh,
    flow_dt_kwh,
    counter_kwh / NULLIF(flow_dt_kwh, 0) AS factor,
    (minutes >= 0.9 * day_minutes
     AND counter_kwh >= 5 AND flow_dt_kwh >= 5
     AND counter_kwh / NULLIF(flow_dt_kwh, 0) BETWEEN 0.8 AND 1.25) AS valid,
    minutes
FROM (
    SELECT
        day, counter_kwh, flow_dt_kwh, minutes,
        extract(epoch FROM ((day + 1)::timestamp AT TIME ZONE local_tz())
                         - (day::timestamp AT TIME ZONE local_tz())) / 60 AS day_minutes
    FROM (
        SELECT
            (bucket AT TIME ZONE local_tz())::date AS day,
            sum(counter_kwh)  AS counter_kwh,
            sum(flow_dt_kwh)  AS flow_dt_kwh,
            sum(seconds) / 60 AS minutes
        FROM heat_intervals_1m
        WHERE counter_kwh IS NOT NULL AND flow_dt_kwh IS NOT NULL
        GROUP BY 1
    ) sums
) d;

-- energy_1m: per minute, kWh consumed (elec_kwh) and delivered (heat_kwh) (ADR-0003).
--   elec_kwh: Electricity Meter counter delta: last counter value in this minute minus the last
--     value in the previous minute that has data, attributed to the minute holding the later sample.
--     NULL when negative (meter reset) or when the previous sample is more than
--     max_counter_gap('electricity_meter') older (data gap).
--   heat_kwh_uncalibrated: Heat Meter flow x Delta T energy of the interval (heat_intervals_1m).
--   heat_kwh = heat_kwh_uncalibrated * calibration_factor: the flow x Delta T heat scaled so that
--     whole days match the Heat Meter's energy counter. The factor of a minute's local day if that
--     day is valid in heat_calibration_daily, else the most recent valid factor of the previous 7
--     days (counting the minute's own day), else 1.0. Looked up for every local day that has heat
--     data, also days without a heat_calibration_daily row.
-- The minute that had no sample at all is absent for that meter; the minute after it is excluded
-- by the gap rule, so a single missed Heat Meter poll drops both minutes from COP.
-- operating_mode comes from heat_pump_1m; 'Unknown' when the minute has no mode.
-- Columns 1-4 are as before the flow x Delta T change (deployed view: append only).
-- Note: window functions prevent time-filter pushdown, so a query scans all energy rows; fine at
-- this data volume (a few rows per minute). The per-day factors are computed once (MATERIALIZED).
CREATE OR REPLACE VIEW energy_1m AS
WITH elec_samples AS (
    SELECT bucket, last_value, last_time,
           lag(last_value) OVER w AS prev_value,
           lag(last_time)  OVER w AS prev_time
    FROM readings_1m
    WHERE metric = 'energy_kwh' AND device = 'electricity_meter'
    WINDOW w AS (ORDER BY bucket)
), elec AS (
    SELECT bucket,
           CASE WHEN last_value >= prev_value
                 AND last_time - prev_time <= max_counter_gap('electricity_meter')
                THEN last_value - prev_value END AS elec_kwh
    FROM elec_samples
), cal AS MATERIALIZED (
    SELECT day, factor FROM heat_calibration_daily WHERE valid
), heat_days AS MATERIALIZED (
    SELECT bucket, flow_dt_kwh, (bucket AT TIME ZONE local_tz())::date AS day
    FROM heat_intervals_1m
), factors AS (
    SELECT d.day,
           coalesce((SELECT c.factor FROM cal c
                      WHERE c.day <= d.day AND c.day >= d.day - 7
                      ORDER BY c.day DESC LIMIT 1),
                    1.0) AS factor
    FROM (SELECT DISTINCT day FROM heat_days) d
), heat AS (
    SELECT i.bucket, i.flow_dt_kwh, f.factor
    FROM heat_days i
    JOIN factors f USING (day)
)
SELECT
    coalesce(h.bucket, e.bucket)                          AS bucket,
    coalesce(m.operating_mode, 'Unknown')::text           AS operating_mode,
    (h.flow_dt_kwh * h.factor)::float8                    AS heat_kwh,
    e.elec_kwh::float8                                    AS elec_kwh,
    h.flow_dt_kwh::float8                                 AS heat_kwh_uncalibrated,
    h.factor::float8                                      AS calibration_factor
FROM heat h
FULL JOIN elec e ON e.bucket = h.bucket
LEFT JOIN heat_pump_1m m ON m.bucket = coalesce(h.bucket, e.bucket);

-- cop_hourly / cop_daily: per bucket and operating_mode, plus a 'Combined' row (operating_mode =
-- 'Combined') over all modes. Minutes without a known mode form their own 'Unknown' row (also part
-- of 'Combined'): no compressor_on sample that minute, so it cannot be classed. Only minutes where BOTH deltas are non-null count. cop = Σheat/Σelec,
-- NULL when Σelec = 0. minutes = number of minutes counted. cop_daily days are Europe/Amsterdam.
CREATE OR REPLACE VIEW cop_hourly AS
SELECT
    time_bucket('1 hour', bucket)        AS bucket,
    coalesce(operating_mode, 'Combined')::text AS operating_mode,
    sum(heat_kwh)                        AS heat_kwh,
    sum(elec_kwh)                        AS elec_kwh,
    sum(heat_kwh) / NULLIF(sum(elec_kwh), 0) AS cop,
    count(*)::int                        AS minutes
FROM energy_1m
WHERE heat_kwh IS NOT NULL AND elec_kwh IS NOT NULL
GROUP BY GROUPING SETS ((time_bucket('1 hour', bucket), operating_mode), (time_bucket('1 hour', bucket)));

CREATE OR REPLACE VIEW cop_daily AS
SELECT
    time_bucket('1 day', bucket, local_tz()) AS bucket,
    coalesce(operating_mode, 'Combined')::text AS operating_mode,
    sum(heat_kwh)                        AS heat_kwh,
    sum(elec_kwh)                        AS elec_kwh,
    sum(heat_kwh) / NULLIF(sum(elec_kwh), 0) AS cop,
    count(*)::int                        AS minutes
FROM energy_1m
WHERE heat_kwh IS NOT NULL AND elec_kwh IS NOT NULL
GROUP BY GROUPING SETS ((time_bucket('1 day', bucket, local_tz()), operating_mode),
                        (time_bucket('1 day', bucket, local_tz())));

GRANT SELECT ON heat_intervals_1m, heat_calibration_daily, energy_1m, cop_hourly, cop_daily TO analytics_ro;
