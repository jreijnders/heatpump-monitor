## Agent skills

### Issue tracker

GitHub Issues via `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five canonical labels (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` + `docs/adr/`. See `docs/agents/domain.md`.

## Heat pump data

Metric catalogue and MQTT contract: `docs/metrics.md`. To query the database (COP, cycling, Heating Curve, Heat Boosters), use the `analyze-heatpump` skill (`.claude/skills/analyze-heatpump/SKILL.md`); it needs the SSH tunnel (`scripts/tunnel.sh`) and `CLAUDE_RO_PASSWORD` exported.
