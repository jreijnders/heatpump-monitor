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
- Use glossary terms (`GLOSSARY.md`): Flow Temperature, Return Temperature, Flow Rate, Operating Mode, Compressor Cycle, Short Cycling, Heating Curve, Curve Shift, Heat Booster, DHW. Metric names/units: `docs/metrics.md`. COP rationale: `docs/adr/0003`.
- Timestamps are `timestamptz` (UTC); present to the user in Europe/Amsterdam.

## Views and functions

`operating_mode` values: `Space Heating`, `DHW`, `Defrost`, `Idle`; `NULL` in `heat_pump_1m`/`booster_1m` when the minute had no `compressor_on` sample (unknown, not Idle); `Unknown` in `energy_1m` and as its own row in the COP views (minutes with no mode; counted in `Combined`, can't be attributed to a mode); `Combined` in the COP views (all modes together). Rule: Defrost > DHW > Space Heating > Idle, majority of the minute. `NULL` only when no defrost flag >= 0.5 and no `compressor_on` sample (Defrost wins even without a compressor sample).

| Name | Columns | Notes |
| --- | --- | --- |
| `data_freshness` | `device, last_time, age, age_seconds` | Last 7 days only. |
| `heat_pump_1m` | `bucket, flow_temp, return_temp, outdoor_temp, room_temp, dhw_temp, target_flow_temp, flow_rate, curve_shift, compressor_on, defrost_active, dhw_active, water_pump_on, error_code, operating_mode` | 1 row/min. Flags are fractions (0..1) of the minute. `error_code` is max. Real-time (includes the unrefreshed tail). |
| `energy_1m` | `bucket, operating_mode, heat_kwh, elec_kwh` | Per-minute counter deltas. Delta is NULL on meter reset (negative) or data gap (previous sample >90 s older, both meters, `max_counter_gap()`). A missed Heat Meter poll drops that minute and the next from COP. |
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

Per-Operating-Mode COP for the last day, hourly:
```sql
SELECT bucket, operating_mode, cop, minutes
FROM cop_hourly WHERE bucket >= now() - interval '1 day' ORDER BY bucket, operating_mode;
```

## Caveats

- Heat Meter energy has Wh resolution; very short intervals give noisy per-hour COP. Prefer daily, and check `minutes`.
- COP needs both counters: minutes missing either delta are dropped, so `minutes` can be far below the elapsed time. Report coverage next to any COP.
- No `heat_pump` data exists before the Gateway cutover to the new flow (only Heat Booster data). No backfill.
- `flow_rate` unit is not yet confirmed (docs/metrics.md, step 9); don't state L/min vs m3/h.
- Heat Meter polls every 60 s, so per-minute mode attribution of its energy is coarse around mode changes.
- Mode and COP views scan all energy counters (window functions); fine now, avoid calling them in tight loops.
- Read-only: never suggest writing; if schema changes are needed, propose a new `db/schema/NNN_*.sql` file instead.
