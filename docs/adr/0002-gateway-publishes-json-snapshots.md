# Gateway publishes timestamped JSON snapshots with canonical metric names

The Gateway publishes one JSON message per device per poll, timestamped when the device is read and using the glossary's metric names (e.g. `flow_temp`, not `lg_water_outlet_temp`). This replaces the original one-value-per-topic, retained, untimestamped scheme. COP requires lining up Heat Meter and Electricity Meter readings in time, and per-topic values arriving separately with ingest-time stamps can't guarantee that. Since we own the Gateway flow, renaming happens there; the ingester only maps names for sources we don't own (the Heat Boosters' firmware).

## Consequences

- Anything else that listened to the old `lg_heatpump/*` topics stops receiving data once the new flow is deployed.
