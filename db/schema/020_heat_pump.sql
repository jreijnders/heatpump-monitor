-- heat_pump_1m: one row per minute with the Heat Pump metrics pivoted (avg per minute) and the
-- derived operating_mode. The pivot and the mode rule live only here.
--
-- Flags (compressor_on, defrost_active, dhw_active, water_pump_on) are minute averages, i.e. the
-- fraction of the minute the flag was on; mode uses the majority (>= 0.5).
-- Mode rule, in order: Defrost, DHW, Space Heating, Idle. Defrost wins even without a compressor
-- sample. operating_mode is NULL only when neither defrost_active >= 0.5 nor a compressor_on
-- sample is present (unknown, not Idle). Missing defrost/dhw flags count as off.
-- error_code is max per minute.
CREATE OR REPLACE VIEW heat_pump_1m AS
SELECT
    p.*,
    CASE
        WHEN coalesce(p.defrost_active, 0) >= 0.5 THEN 'Defrost'
        WHEN p.compressor_on IS NULL THEN NULL
        WHEN p.compressor_on >= 0.5 AND coalesce(p.dhw_active, 0) >= 0.5 THEN 'DHW'
        WHEN p.compressor_on >= 0.5 THEN 'Space Heating'
        ELSE 'Idle'
    END::text AS operating_mode
FROM (
    SELECT
        bucket,
        avg(avg_value) FILTER (WHERE metric = 'flow_temp')        AS flow_temp,
        avg(avg_value) FILTER (WHERE metric = 'return_temp')      AS return_temp,
        avg(avg_value) FILTER (WHERE metric = 'outdoor_temp')     AS outdoor_temp,
        avg(avg_value) FILTER (WHERE metric = 'room_temp')        AS room_temp,
        avg(avg_value) FILTER (WHERE metric = 'dhw_temp')         AS dhw_temp,
        avg(avg_value) FILTER (WHERE metric = 'target_flow_temp') AS target_flow_temp,
        avg(avg_value) FILTER (WHERE metric = 'flow_rate')        AS flow_rate,
        avg(avg_value) FILTER (WHERE metric = 'curve_shift')      AS curve_shift,
        avg(avg_value) FILTER (WHERE metric = 'compressor_on')    AS compressor_on,
        avg(avg_value) FILTER (WHERE metric = 'defrost_active')   AS defrost_active,
        avg(avg_value) FILTER (WHERE metric = 'dhw_active')       AS dhw_active,
        avg(avg_value) FILTER (WHERE metric = 'water_pump_on')    AS water_pump_on,
        max(max_value) FILTER (WHERE metric = 'error_code')       AS error_code
    FROM readings_1m
    WHERE device = 'heat_pump'
    GROUP BY bucket
) p;

GRANT SELECT ON heat_pump_1m TO analytics_ro;
