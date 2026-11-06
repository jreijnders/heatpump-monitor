-- heat_pump_1m: one row per minute with the Heat Pump metrics pivoted (avg per minute) and the
-- derived operating_mode. The pivot and the mode rule live only here.
--
-- Flags (compressor_on, defrost_active, dhw_active, water_pump_on) are minute averages, i.e. the
-- fraction of the minute the flag was on; mode uses the majority (>= 0.5).
-- Mode rule, in order: Defrost, DHW, Space Heating, Idle. Defrost wins even without a compressor
-- sample. operating_mode is NULL only when neither defrost_active >= 0.5 nor a compressor_on
-- sample is present (unknown, not Idle). Missing defrost/dhw flags count as off.
-- error_code is max per minute.
--
-- DEPLOYED VIEW: CREATE OR REPLACE VIEW may only APPEND columns (no rename/reorder/drop). New
-- metrics therefore come after operating_mode, in the second block of the select list.
-- Added later (all minute averages; flags are fractions 0..1, like the flags above): odu_cycle,
-- compressor_hz, discharge_temp, suction_temp, liquid_temp, evaporator_in_temp,
-- evaporator_out_temp, high_pressure, low_pressure, dhw_target_temp, mode_setting, control_method,
-- energy_state, silent_mode, backup_heater_1_on, backup_heater_2_on, dhw_heater_on,
-- dhw_disinfection_active, error_active.
-- flow_rate (Heat Pump) is in L/h, like the Heat Meter flow rate.
-- ASSUMPTION: the Electricity Meter measures the whole Heat Pump including its electric heaters,
-- so elec_kwh includes resistive heating while dhw_heater_on / backup_heater_*_on are > 0.
CREATE OR REPLACE VIEW heat_pump_1m AS
SELECT
    p.bucket, p.flow_temp, p.return_temp, p.outdoor_temp, p.room_temp, p.dhw_temp,
    p.target_flow_temp, p.flow_rate, p.curve_shift, p.compressor_on, p.defrost_active,
    p.dhw_active, p.water_pump_on, p.error_code,
    CASE
        WHEN coalesce(p.defrost_active, 0) >= 0.5 THEN 'Defrost'
        WHEN p.compressor_on IS NULL THEN NULL
        WHEN p.compressor_on >= 0.5 AND coalesce(p.dhw_active, 0) >= 0.5 THEN 'DHW'
        WHEN p.compressor_on >= 0.5 THEN 'Space Heating'
        ELSE 'Idle'
    END::text AS operating_mode,
    p.odu_cycle, p.compressor_hz, p.discharge_temp, p.suction_temp, p.liquid_temp,
    p.evaporator_in_temp, p.evaporator_out_temp, p.high_pressure, p.low_pressure,
    p.dhw_target_temp, p.mode_setting, p.control_method, p.energy_state, p.silent_mode,
    p.backup_heater_1_on, p.backup_heater_2_on, p.dhw_heater_on, p.dhw_disinfection_active,
    p.error_active
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
        avg(avg_value) FILTER (WHERE metric = 'odu_cycle')                AS odu_cycle,
        avg(avg_value) FILTER (WHERE metric = 'compressor_hz')            AS compressor_hz,
        avg(avg_value) FILTER (WHERE metric = 'discharge_temp')           AS discharge_temp,
        avg(avg_value) FILTER (WHERE metric = 'suction_temp')             AS suction_temp,
        avg(avg_value) FILTER (WHERE metric = 'liquid_temp')              AS liquid_temp,
        avg(avg_value) FILTER (WHERE metric = 'evaporator_in_temp')       AS evaporator_in_temp,
        avg(avg_value) FILTER (WHERE metric = 'evaporator_out_temp')      AS evaporator_out_temp,
        avg(avg_value) FILTER (WHERE metric = 'high_pressure')            AS high_pressure,
        avg(avg_value) FILTER (WHERE metric = 'low_pressure')             AS low_pressure,
        avg(avg_value) FILTER (WHERE metric = 'dhw_target_temp')          AS dhw_target_temp,
        avg(avg_value) FILTER (WHERE metric = 'mode_setting')             AS mode_setting,
        avg(avg_value) FILTER (WHERE metric = 'control_method')           AS control_method,
        avg(avg_value) FILTER (WHERE metric = 'energy_state')             AS energy_state,
        avg(avg_value) FILTER (WHERE metric = 'silent_mode')              AS silent_mode,
        avg(avg_value) FILTER (WHERE metric = 'backup_heater_1_on')       AS backup_heater_1_on,
        avg(avg_value) FILTER (WHERE metric = 'backup_heater_2_on')       AS backup_heater_2_on,
        avg(avg_value) FILTER (WHERE metric = 'dhw_heater_on')            AS dhw_heater_on,
        avg(avg_value) FILTER (WHERE metric = 'dhw_disinfection_active')  AS dhw_disinfection_active,
        avg(avg_value) FILTER (WHERE metric = 'error_active')             AS error_active,
        max(max_value) FILTER (WHERE metric = 'error_code')       AS error_code
    FROM readings_1m
    WHERE device = 'heat_pump'
    GROUP BY bucket
) p;

GRANT SELECT ON heat_pump_1m TO analytics_ro;
