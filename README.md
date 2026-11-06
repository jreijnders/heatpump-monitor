# heatpump-monitor

Collects readings from an LG Therma V heat pump, its Heat Meter, Electricity Meter and two Heat Boosters into TimescaleDB, visualises them in Grafana, and lets Claude Code query them read-only. Goal: assess and improve COP (per Operating Mode), Short Cycling, Heating Curve and Heat Booster effectiveness.

- Plan: [FEATURE_PLAN.md](FEATURE_PLAN.md)
- Vocabulary: [GLOSSARY.md](GLOSSARY.md)
- Decisions: [docs/adr/](docs/adr/)
- MQTT contract and metric catalogue: [docs/metrics.md](docs/metrics.md)

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

Grafana and Claude only query the analytics views/functions (`db/schema/020`-`050`), never `readings` directly.

## Setup

```sh
cp .env.example .env        # fill in passwords; set MQTT_URL if needed
docker compose up -d --build
```

- `db-migrate` applies `db/schema/*.sql` on every `up` (all files idempotent) and exits.
- Ports: TimescaleDB `127.0.0.1:${DB_PORT:-5432}` (localhost only; use the SSH tunnel from other machines), Grafana `${GRAFANA_PORT:-3000}`.
- Running a second stack locally next to the server's? Use a different `MQTT_CLIENT_ID` and ports (see comments in `.env.example`), otherwise the two ingesters kick each other off the Broker.
- Backups: `db-backup` writes nightly `pg_dump -Fc` files to `./backups` (keeps `BACKUP_KEEP_DAYS`, default 14).

## Local development

No local Go or Postgres needed; everything runs in Docker.

```sh
scripts/go.sh build ./...                      # Go toolchain in Docker, runs in ingester/
scripts/test-db.sh up dev 55433                # throwaway TimescaleDB with the schema; prints TEST_DATABASE_URL
TEST_DATABASE_URL='postgres://postgres:test@host.docker.internal:55433/heatpump?sslmode=disable' \
  scripts/go.sh test -p 1 ./...
scripts/test-db.sh migrate dev                 # re-apply db/schema after editing SQL
scripts/test-db.sh down dev
```

`-p 1` is required: the store tests temporarily rename the `readings` table, so packages must not run in parallel against the same DB.

## Deploy (server 192.168.1.40)

```sh
git clone <repo> && cd heatpump-monitor
cp .env.example .env && $EDITOR .env
docker compose up -d --build
docker compose ps
```

Update: `git pull && docker compose up -d --build`. Rollback: `git checkout <tag> && docker compose up -d --build`, or `docker compose down` (volumes are kept). Schema files are additive.

## Gateway flow (Node-RED on the Pi)

The flow lives in `nodered/flows.json`.

```sh
export NODERED_URL=http://<pi>:1880      # NODERED_TOKEN=... if adminAuth is on
scripts/pull-flow.sh                     # export the Gateway's flows to nodered/flows.json
scripts/deploy-flow.sh                   # full deploy of nodered/flows.json
```

Edit in the Node-RED editor, then pull and review the diff before committing. Rollback: `git checkout <baseline-commit> -- nodered/flows.json && scripts/deploy-flow.sh`. After cutover to the new topics, clear the old retained ones: `mosquitto_pub -r -n -t lg_heatpump/<topic>`.

## Claude access

1. Start the tunnel (leave running): `scripts/tunnel.sh` (localhost:15432 -> server DB; env `TUNNEL_HOST`, `LOCAL_PORT`, `REMOTE_PORT`).
2. `export CLAUDE_RO_PASSWORD=...` (the server's `.env` value) in the shell that launches Claude Code.
3. Open Claude Code in this repo and approve the project MCP server from `.mcp.json` (`heatpump-db`, [Postgres MCP Pro](https://github.com/crystaldba/postgres-mcp) in a Docker container, `--access-mode=restricted`).
4. Ask questions; the `analyze-heatpump` skill (`.claude/skills/analyze-heatpump/SKILL.md`) lists the views and caveats.

`.mcp.json` connects to `host.docker.internal:15432`. Override with `HEATPUMP_DB_HOST` / `HEATPUMP_DB_PORT` (e.g. `127.0.0.1` / `55432` for a local dev stack on Linux or with host networking). The real guard is the DB role `claude_ro`: SELECT on analytics views only, read-only transactions, 60 s statement timeout.

## Backups and restore

Dumps are custom-format `pg_dump -Fc` files in `./backups`. Restore into an empty database (scratch container shown; same steps for the real DB). TimescaleDB needs the roles first, otherwise the dump's GRANTs fail:

```sh
docker run -d --name restore -e POSTGRES_PASSWORD=x timescale/timescaledb:2.28.2-pg17
docker cp backups/<file>.dump restore:/tmp/db.dump
docker exec -it restore psql -U postgres
```

```sql
CREATE DATABASE heatpump;
-- roles (cluster-wide, not in the dump): create them, or run only the role statements from db/schema/002_roles.sql
CREATE ROLE analytics_ro NOLOGIN;
CREATE ROLE ingester LOGIN; CREATE ROLE grafana_ro LOGIN; CREATE ROLE claude_ro LOGIN;
\c heatpump
CREATE EXTENSION IF NOT EXISTS timescaledb;
SELECT timescaledb_pre_restore();
\! pg_restore -U postgres -d heatpump --no-owner /tmp/db.dump
SELECT timescaledb_post_restore();
```

Then set role passwords (`ALTER ROLE ... PASSWORD`) or re-run `db-migrate`, and check row counts against the source. Memberships and grants are in the dump; `002_roles.sql` re-applies them idempotently.

## Schema changes

Add a new numbered file in `db/schema/`; never edit applied files for continuous aggregates. Plain views/functions use `CREATE OR REPLACE`. Continuous aggregates (`readings_1m`, `readings_1h`) cannot be replaced: drop and recreate them in a new numbered file, and refresh. Every file must stay idempotent; check with `scripts/test-db.sh migrate dev`.
