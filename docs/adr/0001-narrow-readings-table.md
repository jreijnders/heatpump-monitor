# One narrow readings table instead of a table per device

All measurements go into a single TimescaleDB table `readings(time, device, metric, value)` rather than one wide table per device. The Gateway's Node-RED flow will keep changing as LG registers and meter fields are added, and the Heat Boosters publish ~20 fields each; with a narrow table, new metrics need no schema migration. Analytics (COP, cycling, Heating Curve) pivot the metrics they need in SQL views and continuous aggregates, so dashboards and Claude query those rather than raw `readings`.

## Considered Options

- **Wide table per device**: simplest SQL, but every new field requires a migration.
- **Raw JSONB per message**: lossless, but slow and awkward to query.
