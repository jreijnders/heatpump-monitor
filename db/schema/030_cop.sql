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

-- energy_1m: per minute, kWh consumed (elec_kwh) and delivered (heat_kwh), from counter deltas
-- (ADR-0003): last counter value in this minute minus last value in the previous minute that has
-- data, attributed to the minute holding the later sample.
-- A delta is NULL (excluded) when:
--   * negative (meter reset), or
--   * the previous sample is more than max_counter_gap(device) older (data gap / missed poll).
-- So a delta always covers about one minute for both meters, and heat/elec deltas are comparable.
-- The minute that had no sample at all is absent for that meter; the minute after it is excluded
-- by the gap rule, so a single missed Heat Meter poll drops both minutes from COP.
-- operating_mode comes from heat_pump_1m; 'Unknown' when the minute has no mode.
-- Note: window functions prevent time-filter pushdown, so a query scans all energy counters;
-- fine at this data volume (2 metrics, 1 row/min each).
CREATE OR REPLACE VIEW energy_1m AS
WITH counter_samples AS (
    SELECT bucket, device, last_value, last_time,
           lag(last_value) OVER w AS prev_value,
           lag(last_time)  OVER w AS prev_time
    FROM readings_1m
    WHERE metric = 'energy_kwh' AND device IN ('heat_meter', 'electricity_meter')
    WINDOW w AS (PARTITION BY device ORDER BY bucket)
), deltas AS (
    SELECT bucket, device,
           CASE WHEN last_value >= prev_value
                 AND last_time - prev_time <= max_counter_gap(device)
                THEN last_value - prev_value END AS delta
    FROM counter_samples
)
SELECT
    deltas.bucket,
    coalesce(m.operating_mode, 'Unknown')::text AS operating_mode,
    max(deltas.delta) FILTER (WHERE deltas.device = 'heat_meter')        AS heat_kwh,
    max(deltas.delta) FILTER (WHERE deltas.device = 'electricity_meter') AS elec_kwh
FROM deltas
LEFT JOIN heat_pump_1m m USING (bucket)
GROUP BY deltas.bucket, m.operating_mode;

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
    time_bucket('1 day', bucket, 'Europe/Amsterdam') AS bucket,
    coalesce(operating_mode, 'Combined')::text AS operating_mode,
    sum(heat_kwh)                        AS heat_kwh,
    sum(elec_kwh)                        AS elec_kwh,
    sum(heat_kwh) / NULLIF(sum(elec_kwh), 0) AS cop,
    count(*)::int                        AS minutes
FROM energy_1m
WHERE heat_kwh IS NOT NULL AND elec_kwh IS NOT NULL
GROUP BY GROUPING SETS ((time_bucket('1 day', bucket, 'Europe/Amsterdam'), operating_mode),
                        (time_bucket('1 day', bucket, 'Europe/Amsterdam')));

GRANT SELECT ON energy_1m, cop_hourly, cop_daily TO analytics_ro;
