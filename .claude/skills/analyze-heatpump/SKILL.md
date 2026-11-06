---
name: analyze-heatpump
description: Analyse the home heat pump's data in TimescaleDB via the read-only Postgres MCP tool. Use for questions about COP, Compressor Cycles / Short Cycling, Heating Curve tuning, Heat Booster effectiveness, temperatures, operating modes, or data freshness.
---

# Analyse heat pump data

## Prerequisites

- SSH tunnel up: `scripts/tunnel.sh` (localhost:15432 -> server DB).
- `CLAUDE_RO_PASSWORD` exported in the shell that started Claude Code (`.mcp.json` reads it).
- Use the `heatpump-db` MCP server, tool `execute_sql`. Role `claude_ro` is read-only, 60 s statement timeout.
- Not connected? Say so and ask the user to start the tunnel. Do not look for other ways in.

Start with `SELECT * FROM data_freshness` to see which devices report and how recent.

## Rules

- Query ONLY the views/functions below, never `readings` (the pivot and mode logic live in them).
- Use glossary terms (`GLOSSARY.md`): Flow Temperature, Return Temperature, Flow Rate, Operating Mode, Compressor Cycle, Compressor Frequency, DHW Heater, Backup Heater, Calibration Factor, Short Cycling, Heating Curve, Curve Shift, Heat Booster, DHW. Metric names/units: `docs/metrics.md`. COP rationale: `docs/adr/0003`.
- Timestamps are `timestamptz` (UTC); present to the user in Europe/Amsterdam.

## Views and functions

`operating_mode` values: `Space Heating`, `DHW`, `Defrost`, `Idle`; `NULL` in `heat_pump_1m`/`booster_1m` when the minute had no `compressor_on` sample (unknown, not Idle); `Unknown` in `energy_1m` and as its own row in the COP views (minutes with no mode; counted in `Combined`, can't be attributed to a mode); `Combined` in the COP views (all modes together). Rule: Defrost > DHW > Space Heating > Idle, majority of the minute. `NULL` only when no defrost flag >= 0.5 and no `compressor_on` sample (Defrost wins even without a compressor sample).

| Name | Columns | Notes |
| --- | --- | --- |
| `data_freshness` | `device, last_time, age, age_seconds` | Last 7 days only. |
| `heat_pump_1m` | `bucket, flow_temp, return_temp, outdoor_temp, room_temp, dhw_temp, target_flow_temp, flow_rate, curve_shift, compressor_on, defrost_active, dhw_active, water_pump_on, error_code, operating_mode, odu_cycle, compressor_hz, discharge_temp, suction_temp, liquid_temp, evaporator_in_temp, evaporator_out_temp, high_pressure, low_pressure, dhw_target_temp, mode_setting, control_method, energy_state, silent_mode, backup_heater_1_on, backup_heater_2_on, dhw_heater_on, dhw_disinfection_active, error_active, delta_t` | 1 row/min, minute averages. `delta_t` = Delta T across the Heat Pump (flow_temp − return_temp); ~5 K is the Space Heating target. Flags (`*_on`, `*_active`, `silent_mode`, ...) are fractions (0..1) of the minute. `error_code` is max. `flow_rate` is L/h. `compressor_hz` = Compressor Frequency. Columns after `operating_mode` exist only since the Gateway publishes them (NULL before). Real-time (includes the unrefreshed tail). |
| `energy_1m` | `bucket, operating_mode, heat_kwh, elec_kwh, heat_kwh_uncalibrated, calibration_factor` | Per minute. `elec_kwh`: Electricity Meter counter delta. `heat_kwh_uncalibrated`: Heat Meter flow rate x Delta T x 1.163 Wh/(L K), trapezoid over its 60 s samples. `heat_kwh` = uncalibrated x `calibration_factor` (per-day factor, see `heat_calibration_daily`; fallback latest valid of previous 7 days, else 1.0). NULL on meter reset (elec), data gap (previous sample >90 s older, `max_counter_gap()`) or missing flow/temperatures (heat). A missed Heat Meter poll drops that minute and the next from COP. |
| `heat_calibration_daily` | `day, counter_kwh, flow_dt_kwh, factor, valid, minutes` | Per Europe/Amsterdam day: Heat Meter counter delta vs flow x Delta T energy over the same intervals. `valid` = day >= 90 % covered, both sums >= 5 kWh, factor 0.8-1.25; only valid factors are applied. Check drift here. |
| `heat_intervals_1m` | `bucket, flow_dt_kwh, counter_kwh, seconds` | Internal building block of the two views above; rarely needed. |
| `cop_hourly` | `bucket, operating_mode, heat_kwh, elec_kwh, cop, minutes` | Only minutes where BOTH deltas exist. Has a `Combined` row per bucket. `cop` NULL if elec = 0. |
| `cop_daily` | same | Days are Europe/Amsterdam. |
| `compressor_cycles(p_from, p_to)` | `cycle_start, cycle_end, duration` | From raw `compressor_on`. Open cycle: `cycle_end`/`duration` NULL. Returns cycles overlapping `[p_from, p_to)`; a cycle already running at `p_from` keeps its real start (24 h look-back). Keep ranges bounded (days to weeks). |
| `power_1m` | `bucket, device, power_w` | Per-minute avg power (W). Used by Grafana. |
| `heating_curve_1h` | `bucket, flow_temp, target_flow_temp, outdoor_temp, room_temp, curve_shift, minutes` | Space Heating minutes only. |
| `booster_1m` | `bucket, device, flow_temp, return_temp, delta_t, ambient_temp, fan_speed, fan_boost, rssi, heat_pump_flow_temp, operating_mode` | `device` = every `heat_booster_*` device (e.g. `heat_booster_links`). `delta_t` = flow - return. |

`readings_1m` / `readings_1h` are continuous aggregates behind these views (Grafana uses views only); they are real-time (materialized + recent raw), so the latest minutes are included.

## Example queries

COP in Space Heating last week vs outdoor temperature:
```sql
SELECT c.bucket::date AS day, c.cop, c.heat_kwh, c.elec_kwh,
       avg(h.outdoor_temp) AS outdoor_avg
FROM cop_daily c
JOIN heat_pump_1m h
  ON time_bucket('1 day', h.bucket, 'Europe/Amsterdam') = c.bucket
WHERE c.operating_mode = 'Space Heating'
  AND c.bucket >= now() - interval '7 days'
GROUP BY c.bucket, c.cop, c.heat_kwh, c.elec_kwh
ORDER BY c.bucket;
```
(Better still: restrict `h` to `operating_mode = 'Space Heating'` for outdoor temp during heating.)

Short Cycling per day (cycles under 10 min):
```sql
SELECT time_bucket('1 day', cycle_start, 'Europe/Amsterdam') AS day,
       count(*) AS cycles,
       count(*) FILTER (WHERE duration < interval '10 minutes') AS short_cycles,
       avg(duration) AS avg_duration
FROM compressor_cycles(now() - interval '14 days', now())
GROUP BY 1 ORDER BY 1;
```

Heating Curve fit (actual vs target Flow Temperature by outdoor temp band):
```sql
SELECT round(outdoor_temp)::int AS outdoor_c,
       avg(flow_temp) AS flow, avg(target_flow_temp) AS target,
       avg(room_temp) AS room, sum(minutes) AS minutes
FROM heating_curve_1h
WHERE bucket >= now() - interval '30 days'
GROUP BY 1 ORDER BY 1;
```

Heat Booster Delta T vs fan speed:
```sql
SELECT device, round(fan_speed / 5) * 5 AS fan_pct,
       avg(delta_t) AS avg_delta_t, avg(flow_temp) AS flow, count(*) AS minutes
FROM booster_1m
WHERE bucket >= now() - interval '14 days' AND operating_mode = 'Space Heating'
GROUP BY 1, 2 ORDER BY 1, 2;
```

Calibration Factor drift (should hover near 1.0, valid days only applied):
```sql
SELECT day, counter_kwh, flow_dt_kwh, round(factor::numeric, 3) AS factor, valid, round(minutes) AS minutes
FROM heat_calibration_daily ORDER BY day DESC LIMIT 30;
```

Compressor Frequency and discharge temperature per Operating Mode, hourly:
```sql
SELECT time_bucket('1 hour', bucket) AS hour, operating_mode,
       avg(compressor_hz) AS hz, avg(discharge_temp) AS discharge, avg(high_pressure) AS high_p
FROM heat_pump_1m
WHERE bucket >= now() - interval '2 days' AND compressor_hz IS NOT NULL
GROUP BY 1, 2 ORDER BY 1, 2;
```

Per-Operating-Mode COP for the last day, hourly:
```sql
SELECT bucket, operating_mode, cop, minutes
FROM cop_hourly WHERE bucket >= now() - interval '1 day' ORDER BY bucket, operating_mode;
```

## Caveats

- The Heat Meter counter has 1 kWh resolution, so per-minute heat is flow x Delta T scaled by a per-day Calibration Factor (ADR-0003). Per-mode shape is as good as flow x Delta T; absolute level is calibrated per day, not per mode. Prefer hourly/daily COP; check `minutes`.
- The factor of the current (incomplete) day is borrowed from earlier days, so today's COP is provisional. A factor far from 1.0 or invalid days in `heat_calibration_daily` hints at flow-rate/temperature sensor problems.
- COP needs both: minutes missing a heat interval (no flow/temperatures, gap) or an electricity delta are dropped, so `minutes` can be far below the elapsed time. Report coverage next to any COP. Heat before the Gateway published flow rate and temperatures is NULL.
- ASSUMPTION: the Electricity Meter measures the whole Heat Pump including its electric heaters. When `dhw_heater_on` (DHW Heater) or `backup_heater_1_on`/`backup_heater_2_on` (Backup Heater) are > 0, `elec_kwh` includes resistive heating (COP ~1 for that share): check these flags before blaming the Heat Pump for a low COP, esp. in DHW mode and during disinfection (`dhw_disinfection_active`).
- No `heat_pump` data exists before the Gateway cutover to the new flow (only Heat Booster data). No backfill.
- `heat_pump_1m.flow_rate` is L/h (like the Heat Meter's flow rate).
- Heat Meter polls every 60 s; a power step is split over two neighbouring minutes (trapezoid), so per-minute mode attribution is slightly smeared around mode changes.
- Mode and COP views scan all energy counters (window functions); fine now, avoid calling them in tight loops.
- Read-only: never suggest writing; if schema changes are needed, propose a new `db/schema/NNN_*.sql` file instead.
