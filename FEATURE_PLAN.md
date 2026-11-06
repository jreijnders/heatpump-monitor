# Feature plan: heatpump-monitor v1 (ingest → TimescaleDB → Grafana → Claude)

## Summary
Store every reading from the Heat Pump, Heat Meter, Electricity Meter and both Heat Boosters in TimescaleDB, visualise performance in Grafana, and let Claude Code query the data read-only. The goal is to assess and improve the home installation's COP. That covers per Operating Mode COP, Short Cycling, Heating Curve tuning and Heat Booster effectiveness. The Gateway's Node-RED flow moves into this repo and switches to the timestamped-JSON contract (ADR-0002). Vocabulary follows `GLOSSARY.md`; design decisions are in `docs/adr/0001–0003`.

## Success criteria
- Gateway publishes `heatpump-monitor/gateway/{heat_pump,electricity_meter}` every 10 s and `.../heat_meter` every 60 s as JSON with a read-time `ts`.
- Ingester writes those plus both Heat Boosters' measurements into `readings`. In normal operation, the gap between a reading's `time` and the moment it lands in the DB stays under 5 s.
- A restart of the ingester or DB loses no Gateway readings (QoS 1 + persistent session).
- `cop_daily` returns COP per day per Operating Mode (Space Heating, DHW) and combined. The values match a hand calculation from the meters' counter readings for a sample day.
- `compressor_cycles(from, to)` lists cycles with start, end and duration. Short Cycling is visible in Grafana.
- Grafana at `http://192.168.1.40:3000` shows the Performance dashboard (Live, COP, Cycling, Heating Curve, Heat Boosters rows), provisioned from repo files.
- From this repo, with the SSH tunnel up, Claude Code can answer "what was COP in Space Heating last week vs outdoor temp?" via the read-only role and can't write.
- Nightly `pg_dump` file appears in the backup volume; a restore into a scratch container succeeds.

## Out of scope
- Cooling mode, heating circuit 2, Backup Heater tracking: not used (heater disabled, one circuit).
- Home Assistant sensors: room temp comes from the Heat Pump's sensor + Booster Ambient Temperature.
- Scheduled Claude reports / Claude API usage: interactive only for now.
- Alerting (Grafana alerts on error_code, data gaps). Easy follow-up once dashboards exist.
- Controlling the Heat Pump (writing setpoints/Heating Curve) from this system. Read-only monitoring.
- CI / image registry: build on the server (decided).
- Backfilling history: there is none (old topics were retained-only snapshots).

## Architecture

```
 Pi (Gateway, Node-RED)                     Server 192.168.1.40 (docker compose)
 ┌─────────────────────────┐   MQTT QoS1   ┌──────────┐   ┌──────────┐   ┌─────────────┐
 │ Modbus: Heat Pump  10s  │──────────────▶│ Mosquitto│──▶│ ingester │──▶│ timescaledb │◀─ grafana :3000
 │ Modbus: Elec Meter 10s  │  JSON + ts    │ (exists) │   │  (Go)    │   │  readings   │
 │ M-Bus:  Heat Meter 60s  │               └──────────┘   └──────────┘   │  caggs/views│◀─ db-backup (nightly)
 └─────────────────────────┘                    ▲                        └─────────────┘
 Heat Boosters (own firmware) ─ heatbooster/<name>/<field> ┘          127.0.0.1:5432 ◀─ ssh -L ◀─ Claude Code (MCP, claude_ro)
```

**MQTT contract** (documented in `docs/metrics.md`, the single source of truth for metric names and units):
- Topic `heatpump-monitor/gateway/<device>`, QoS 1, not retained.
- Payload `{"ts": "<RFC3339 UTC>", "metrics": {"<metric>": <number>, ...}}`.
- Devices: `heat_pump`, `electricity_meter`, `heat_meter`. Heat Boosters map to `heat_booster_links`, `heat_booster_rechts`.
- Gateway publishes **raw status flags** (`compressor_on`, `defrost_active`, `dhw_active`, `water_pump_on`). Operating Mode is **derived in SQL**, so rules can be fixed and re-applied to history.

**Ingester** (`ingester/`, Go, deps: paho.mqtt.golang, pgx/v5):
- `decode` (pure): `(topic, payload, retained, receivedAt) → []Reading`.
  - Gateway JSON uses its `ts`. It falls back to receivedAt if `ts` is missing, and rejects `ts` more than 5 min in the future.
  - Heat Booster topics: allowlisted field map, stamped with receivedAt. Retained messages are skipped, and so are unknown fields.
- `store`: batches rows (flush at 500 rows or 2 s). Writes with `INSERT … SELECT unnest(...) ON CONFLICT DO NOTHING` (idempotent on `(device, metric, time)`). Retries with backoff and a bounded buffer.
- `main`: env config, slog, persistent session (`CleanSession=false`, fixed client ID), subscribes `heatpump-monitor/gateway/+` and `heatbooster/+/+`.

**DB** (`db/schema/*.sql`, idempotent, applied in filename order by a one-shot `db-migrate` compose service using `psql` on every `up`):
- `readings(time, device, metric, value)` hypertable, unique `(device, metric, time)`. Compression segmentby `device, metric`, after 7 days. No retention policy.
- Roles: `ingester` (INSERT/SELECT on readings), `grafana_ro`, `claude_ro` (SELECT on readings + analytics views/functions only).
- Continuous aggregates (narrow, generic): `readings_1m` (avg/min/max/last per device+metric), `readings_1h` hierarchical on `readings_1m`.
- Views/functions:
  - `heat_pump_1m`: pivot + derived `operating_mode`.
  - `energy_1m`: counter deltas per minute for heat and electricity via `lag(last)`, joined to mode.
  - `cop_hourly`, `cop_daily`: Σheat/Σelec per mode + combined.
  - `compressor_cycles(from, to)`: edge detection on raw `compressor_on`.
  - `heating_curve_1h`: flow_temp vs outdoor_temp vs room_temp, Space Heating only.
  - `booster_1m`.

**Seams**: MQTT contract (Gateway ↔ ingester), `readings` table (ingester ↔ SQL layer), analytics views (SQL ↔ Grafana/Claude). Grafana and Claude only touch views/functions, never `readings` directly, so the pivot logic lives in one place.

## Files to create
| Path | Purpose |
| --- | --- |
| `compose.yaml` | timescaledb, db-migrate, ingester, grafana, db-backup services |
| `.env.example` | Passwords for postgres/ingester/grafana_ro/claude_ro, Grafana admin, MQTT_URL |
| `.gitignore` | `.env`, `ingester/bin/`, backup dir |
| `docs/metrics.md` | MQTT contract + metric catalogue per device (name, unit, source register) |
| `db/schema/001_readings.sql` | Extension, hypertable, unique index, compression policy |
| `db/schema/002_roles.sql` | Roles + grants (passwords via psql `-v` vars) |
| `db/schema/010_aggregates.sql` | `readings_1m`, `readings_1h` caggs + refresh policies |
| `db/schema/020_heat_pump.sql` | `heat_pump_1m` pivot + `operating_mode` derivation |
| `db/schema/030_cop.sql` | `energy_1m`, `cop_hourly`, `cop_daily` |
| `db/schema/040_cycles.sql` | `compressor_cycles(from, to)` |
| `db/schema/050_curve_boosters.sql` | `heating_curve_1h`, `booster_1m` |
| `db/migrate.sh` | Runs `psql -v ON_ERROR_STOP=1` over `db/schema/*.sql` in order |
| `ingester/go.mod` | Module `heatpump-monitor/ingester` |
| `ingester/cmd/ingester/main.go` | Config from env, wiring, signal handling |
| `ingester/internal/reading/decode.go` | Topic/payload → `[]Reading`; Heat Booster field map |
| `ingester/internal/reading/decode_test.go` | Table-driven decode tests |
| `ingester/internal/store/store.go` | Batching writer (pgx) |
| `ingester/internal/store/store_test.go` | Integration tests against real TimescaleDB (`TEST_DATABASE_URL`) |
| `ingester/internal/analytics_test/analytics_test.go` | Fixture readings → assert COP/mode/cycles views (`TEST_DATABASE_URL`) |
| `ingester/Dockerfile` | Multi-stage build → distroless static |
| `grafana/provisioning/datasources/timescale.yaml` | Datasource as `grafana_ro` |
| `grafana/provisioning/dashboards/dashboards.yaml` | File provider for `grafana/dashboards/` |
| `grafana/dashboards/performance.json` | Rows: Live, COP, Cycling, Heating Curve, Heat Boosters, Data freshness |
| `scripts/backup.sh` | Nightly `pg_dump -Fc` loop, keep 14 days (runs in timescale image) |
| `scripts/tunnel.sh` | `ssh -N -L 15432:127.0.0.1:5432 jeroen@192.168.1.40` |
| `scripts/pull-flow.sh` | GET `$NODERED_URL/flows` → `nodered/flows.json` (pretty-printed for diffs) |
| `scripts/deploy-flow.sh` | POST `nodered/flows.json` to `$NODERED_URL/flows` (full deploy, bearer token if adminAuth) |
| `nodered/flows.json` | Gateway flow (baseline export, then rewritten) |
| `.mcp.json` | Postgres MCP server → `postgresql://claude_ro@127.0.0.1:15432/heatpump` (password from env) |
| `.claude/skills/analyze-heatpump/SKILL.md` | How Claude should analyse: views, metric catalogue pointer, glossary, example queries, caveats |
| `README.md` | Setup, deploy, tunnel, flow deploy, restore |

## Files to modify
| Path | Change |
| --- | --- |
| `GLOSSARY.md` | Only if new domain terms surface (e.g. while mapping LG registers) |
| `CLAUDE.md` | Point to `docs/metrics.md` and the analyze-heatpump skill |

## Implementation steps
Steps 1–8 don't need the Pi; Heat Booster data is live and Gateway messages can be faked with `mosquitto_pub`.

1. **Stack skeleton + schema.** `compose.yaml` (timescaledb bound to `127.0.0.1:5432`, db-migrate), `.env.example`, `.gitignore`, `db/schema/001`, `002`, `db/migrate.sh`. Verify `docker compose up` locally creates the hypertable and roles; re-running is a no-op.
2. **Contract doc.** `docs/metrics.md`: topics, payload shape, every metric per device with unit. Heat Booster mapping:
   - `temp-inlet` → `flow_temp`, `temp-outlet` → `return_temp`, `temp-ambient` → `ambient_temp`.
   - `fan-speed` → `fan_speed`, `fan-boostmode` → `fan_boost`, `rssi` → `rssi`.
   - `temp-delta-io` is dropped (derivable).
3. **Decode (TDD).** `internal/reading` with tests first (see checklist).
4. **Store + main + Dockerfile.** Batching writer, MQTT wiring, compose `ingester` service. Locally against the real Broker: Heat Booster rows land; a hand-published Gateway JSON lands with its own `ts`.
5. **Analytics SQL (TDD with fixtures).**
   - Files `010`–`050`.
   - `operating_mode` rule, in order: defrost_active → Defrost; else compressor_on and dhw_active → DHW; else compressor_on → Space Heating; else Idle. Mode per minute = majority in that minute (via avg of flags in `readings_1m`).
   - COP views ignore intervals where either counter delta is negative or missing (meter reset / gap).
6. **Grafana.** Provisioning + `performance.json` built in the UI against fixture/live data, exported to the repo, then loaded from the repo files. Datasource uses `grafana_ro`.
7. **Backups.** `db-backup` service using the same timescale image running `scripts/backup.sh`. Document restore (`timescaledb_pre_restore()` / `post_restore()`) in README.
8. **Claude access.** `scripts/tunnel.sh`, `.mcp.json`, `analyze-heatpump` skill, CLAUDE.md pointer. Verify a write as `claude_ro` fails.
9. **Gateway baseline** (Pi on). `scripts/pull-flow.sh`, commit the exported flow unchanged. Record:
   - the LG registers actually read
   - Electricity Meter model and registers
   - Multical M-Bus fields
   - `lg_current_flow_rate` unit

   …in `docs/metrics.md`. Check whether Home Assistant subscribes to `lg_heatpump/*`.
10. **Gateway rewrite.**
    - In the Node-RED editor on the Pi: 10 s Modbus poll for the Heat Pump + Electricity Meter, 60 s M-Bus poll for the Heat Meter.
    - Add the status-flag registers. Drop `lg_backup_heater_temp` and the per-value topics. Build JSON per device per poll with `ts`, published QoS 1.
    - Pull the flow into the repo, review the diff, commit.
    - Clear old retained topics: `mosquitto_pub -r -n -t lg_heatpump/<each>`.
11. **Server deploy.** User installs an SSH key on .40. `git clone`, `cp .env.example .env`, `docker compose up -d --build`. Run the E2E checklist.

## Maintainability checks
- [ ] Ingester packages stay small and single-purpose (`reading` = pure decode, `store` = DB writes, `main` = wiring); no file > ~300 lines.
- [ ] No `utils`/`helpers` package; metric names come from `docs/metrics.md`, and the Go field map is the only code copy of Heat Booster names.
- [ ] No interface introduced for MQTT or DB "for testability" without a second caller. Decode is pure, store is tested against real Timescale.
- [ ] New dependencies: `github.com/eclipse/paho.mqtt.golang`, `github.com/jackc/pgx/v5`, a Postgres MCP server (chosen in step 8). Nothing else.
- [ ] Only `cmd/ingester` is a binary; everything else under `internal/`.
- [ ] Every `db/schema/*.sql` file is idempotent (`IF NOT EXISTS`, `CREATE OR REPLACE`); re-running `db-migrate` on a populated DB changes nothing.
- [ ] Grafana panels and Claude query views/functions, never pivot `readings` themselves.
- [ ] Functions < ~50 lines, ≤ 3 nesting levels.

## Stale-code removal
The repo has no code yet; stale items are on the Broker and Gateway:
- [ ] Gateway flow: per-value `lg_heatpump/*` MQTT-out nodes, replaced by JSON-per-device publish (step 10).
- [ ] Gateway flow: `lg_backup_heater_temp` read (returns garbage 6488.7; Backup Heater disabled).
- [ ] Gateway flow: the `g_target_temp_circuit1` topic (typo'd duplicate of target temp, publishing 48 vs 39). Find out which register it is in step 9, then keep it under a canonical name or drop it.
- [ ] Broker: retained `lg_heatpump/*` messages, cleared after cutover (step 10).
- [ ] Circuit 2 shift value read (`lg_shift_value_in_auto_mode_circuit2`): out of scope, drop.

Searched: whole repo (only agent docs), live Broker topics via `mosquitto_sub -t '#'`.

## Duplicate prevention
| Functionality needed | Existing code reused | If new, why |
| --- | --- | --- |
| MQTT client | paho.mqtt.golang | — |
| Postgres driver / batching | pgx/v5 | — |
| Schema migrations | plain `psql` in timescale image | golang-migrate etc. unnecessary for a handful of idempotent files |
| Backups | `pg_dump` in timescale image | avoids a third-party backup image |
| Pivots / COP / cycles | Timescale caggs + SQL views | One place shared by Grafana and Claude |
| Device polling | Existing Node-RED flow on the Pi | Kept; only its output format changes |
| Heat Booster ingest | none | Firmware publishes per-field topics; needs mapping |

- [ ] Searched for similar functionality with terms: repo empty apart from `docs/agents/*`, `GLOSSARY.md`, `docs/adr/*`.
- [ ] No copy-pasted block > ~10 lines (watch the pivot SQL: shared via `readings_1m`, not duplicated per view).

## Unit test checklist
Go tests live next to the code (`*_test.go`), table-driven, `t.Run` subtests.

**Decode (`internal/reading/decode_test.go`)**
- [ ] Gateway `heat_pump` JSON with 3 metrics → 3 Readings, `time` = payload `ts`, device `heat_pump`.
- [ ] Gateway JSON without `ts` → stamped with receivedAt.
- [ ] Gateway JSON with `ts` 10 min in the future → rejected (error, no readings).
- [ ] Malformed JSON / `metrics` not an object / non-numeric value → error, no partial readings.
- [ ] Unknown device in topic → error.
- [ ] Empty `metrics` → zero readings, no error.
- [ ] `heatbooster/Heatbooster-rechts/temp-inlet` = `41.28` → device `heat_booster_rechts`, metric `flow_temp`, value 41.28 (case-insensitive device name).
- [ ] `heatbooster/heatbooster-links/ssid` → ignored (not in allowlist).
- [ ] Heat Booster non-numeric value on allowlisted field → error.
- [ ] Retained message (either source) → skipped.
- [ ] Unrelated topic (`homeassistant/humidor/state`) → ignored.

**Store (`internal/store/store_test.go`, needs `TEST_DATABASE_URL`)**
- [ ] Batch of 1000 readings inserted; count matches.
- [ ] Same reading twice → one row (ON CONFLICT).
- [ ] Flush on timer with < batch size rows.
- [ ] DB down during flush → retries, rows written once DB returns; buffer cap drops oldest and logs a count.
- [ ] Shutdown (context cancel) flushes pending rows.

**Analytics SQL (`analytics_test.go`, fixtures inserted, caggs refreshed with `CALL refresh_continuous_aggregate`)**
- [ ] Mode derivation: each flag combination → expected `operating_mode`; Defrost wins over DHW.
- [ ] COP: 1 h Space Heating with heat counter +3.000 kWh and elec +0.750 kWh → `cop_hourly` = 4.0.
- [ ] COP split: 30 min DHW + 30 min Space Heating in one hour → separate per-mode COPs, combined = Σ/Σ.
- [ ] Counter reset (negative delta) and 10-min data gap → interval excluded, no division by zero.
- [ ] `compressor_cycles`: on 10:00–10:04, off, on 10:20–11:00 → two cycles, 4 min and 40 min. Cycle open at range end → reported with null end.
- [ ] Heat Meter at 60 s vs Heat Pump at 10 s → deltas attributed to the right minute.

## End-to-end local verification checklist
Local (Mac, before server deploy):
- [ ] `cp .env.example .env && docker compose up -d --build`: all services healthy, `db-migrate` exits 0.
- [ ] `docker compose run --rm db-migrate` again → exits 0, no changes.
- [ ] `docker compose logs ingester` shows connected + subscribed; within 30 s `SELECT device, metric, count(*) FROM readings GROUP BY 1,2` shows `heat_booster_*` rows (live Broker).
- [ ] `docker run --rm eclipse-mosquitto:2 mosquitto_pub -h 192.168.1.40 -q 1 -t heatpump-monitor/gateway/heat_pump -m '{"ts":"2026-11-06T12:00:00Z","metrics":{"flow_temp":35.2}}'` → row with time 12:00:00Z.
- [ ] Publish malformed JSON → ingester logs a warning and keeps running.
- [ ] `docker compose stop ingester`, publish 3 QoS1 messages, `start ingester` → all 3 rows appear.
- [ ] Open `http://localhost:3000`, log in, Performance dashboard loads from provisioning; Heat Booster panels show data.
- [ ] `psql -U claude_ro … -c "DELETE FROM readings"` → permission denied.

Server + Gateway (after steps 10–11):
- [ ] `docker compose ps` on .40: all up; Grafana at `http://192.168.1.40:3000`.
- [ ] Live row: `heat_pump` and `electricity_meter` freshness < 15 s, `heat_meter` < 90 s.
- [ ] Watch a DHW run: Operating Mode switches to DHW in Grafana; after it, `cop_hourly` shows a DHW row.
- [ ] Hand-check one hour: heat and elec counter values at the hour boundaries from raw `readings` → COP equals `cop_hourly`.
- [ ] `scripts/tunnel.sh` + Claude Code in repo: "COP per day last week, per mode" answered via MCP.
- [ ] Next morning: dump file in backup volume; restore into a scratch timescale container, row counts match.
- [ ] Reboot server → stack comes back (`restart: unless-stopped`), no lost Gateway data across the outage.
- [ ] Home Assistant: nothing that matters broke from the `lg_heatpump/*` removal.

## Risks and unknowns
- **LG status registers** (compressor, defrost, 3-way valve/DHW) not yet confirmed for this model. Resolve in step 9 from the existing flow + LG Therma V Modbus map; mode derivation depends on them. Fallback: infer DHW from 3-way valve or DHW temp rise.
- **`lg_current_flow_rate` unit** (1.43: L/min ×? or m³/h). Verify against the Multical flow in step 9. COP doesn't depend on it (uses the Heat Meter).
- **Pi clock**: Gateway `ts` depends on NTP on the Pi. Check `timedatectl` in step 9; ingester rejects future timestamps.
- **Battery-powered Multical**: 60 s M-Bus polling. Confirm Kamstrup's allowed read rate for this model; slow to 120–300 s if needed (COP from counters is unaffected, only per-minute mode attribution coarsens).
- **Home Assistant may consume `lg_heatpump/*`**. Check HA config before step 10; if needed, add HA MQTT sensors on the new JSON topics.
- **Changing a continuous aggregate** later needs drop + recreate (not `CREATE OR REPLACE`). Handle with explicit numbered SQL files; keep caggs generic (`readings_1m` narrow) so they rarely change.
- **Inserting into compressed chunks** with `ON CONFLICT`: only late data older than 7 days hits this. Needs TimescaleDB ≥ 2.11; pin a recent image.
- **Postgres MCP server choice**: the reference `@modelcontextprotocol/server-postgres` is archived. Pick a maintained read-only-capable one in step 8. The DB role is the real guard regardless.
- **Ingester crash loses ≤ 2 s of buffered rows**. Paho acks on handler return, before the DB write. Accepted for v1.
- **No SSH access to .40 yet** blocks steps 8 and 11. User adds a key.

## Rollout / rollback
- Steps 1–8 run alongside today's setup without affecting anything; the Gateway is untouched until step 10.
- Gateway cutover (step 10) is the only breaking change. Before deploying, keep the step-9 baseline commit. Rollback = `git checkout <baseline> -- nodered/flows.json && scripts/deploy-flow.sh`.
- Server stack rollback: `docker compose down` (volume kept) or `git checkout <tag> && docker compose up -d --build`. Schema files are additive; no destructive migrations in v1.
