-- heating_curve_1h: hourly averages over Space Heating minutes only (Heating Curve tuning).
-- minutes = number of Space Heating minutes in the hour.
CREATE OR REPLACE VIEW heating_curve_1h AS
SELECT
    time_bucket('1 hour', bucket) AS bucket,
    avg(flow_temp)        AS flow_temp,
    avg(target_flow_temp) AS target_flow_temp,
    avg(outdoor_temp)     AS outdoor_temp,
    avg(room_temp)        AS room_temp,
    avg(curve_shift)      AS curve_shift,
    count(*)::int         AS minutes
FROM heat_pump_1m
WHERE operating_mode = 'Space Heating'
GROUP BY 1;

-- booster_1m: per Heat Booster (every device named heat_booster_*) per minute.
-- delta_t = flow_temp - return_temp. heat_pump_flow_temp and operating_mode come from heat_pump_1m.
CREATE OR REPLACE VIEW booster_1m AS
SELECT
    b.bucket,
    b.device,
    b.flow_temp,
    b.return_temp,
    b.flow_temp - b.return_temp AS delta_t,
    b.ambient_temp,
    b.fan_speed,
    b.fan_boost,
    b.rssi,
    h.flow_temp      AS heat_pump_flow_temp,
    h.operating_mode AS operating_mode
FROM (
    SELECT
        bucket, device,
        avg(avg_value) FILTER (WHERE metric = 'flow_temp')    AS flow_temp,
        avg(avg_value) FILTER (WHERE metric = 'return_temp')  AS return_temp,
        avg(avg_value) FILTER (WHERE metric = 'ambient_temp') AS ambient_temp,
        avg(avg_value) FILTER (WHERE metric = 'fan_speed')    AS fan_speed,
        avg(avg_value) FILTER (WHERE metric = 'fan_boost')    AS fan_boost,
        avg(avg_value) FILTER (WHERE metric = 'rssi')         AS rssi
    FROM readings_1m
    WHERE device LIKE 'heat\_booster\_%'
    GROUP BY bucket, device
) b
LEFT JOIN heat_pump_1m h USING (bucket);

-- data_freshness: last reading per device and its age. Looks at the last 7 days only (cheap);
-- a device silent for longer is absent.
CREATE OR REPLACE VIEW data_freshness AS
SELECT
    device,
    max(time)                                         AS last_time,
    now() - max(time)                                 AS age,
    extract(epoch FROM now() - max(time))::float8     AS age_seconds
FROM readings
WHERE time > now() - INTERVAL '7 days'
GROUP BY device;

GRANT SELECT ON heating_curve_1h, booster_1m, data_freshness TO analytics_ro;
