# MQTT contract and metric catalogue

Single source of truth for topics, payloads, metric names and units. Terms follow `GLOSSARY.md`; rationale in `docs/adr/0002`. The only code copy of the Heat Booster names is the field map in `ingester/internal/reading/decode.go`.

## Gateway topics

- Topic: `heatpump-monitor/gateway/<device>`, `<device>` one of `heat_pump`, `electricity_meter`, `heat_meter`.
- QoS 1, **not retained**.
- Payload (one message per device per poll):

```json
{"ts": "2026-11-06T12:00:00Z", "metrics": {"flow_temp": 35.2, "compressor_on": 1}}
```

- `ts`: RFC3339 UTC, the moment the device was read. `metrics`: flat object of metric name to number.
- Poll interval: `heat_pump` and `electricity_meter` 10 s, `heat_meter` 60 s (battery powered).
- The Gateway publishes raw status flags (0/1). Operating Mode is derived in SQL, never by the Gateway.

## Heat Booster topics

- Published by the Heat Booster firmware (not ours): `heatbooster/<Heatbooster-links|Heatbooster-rechts>/<field>`, plain-text value, one topic per field, every ~10 s (`interval`).
- Device name match is case-insensitive (observed: `heatbooster-links` and `Heatbooster-rechts`). Mapped to `heat_booster_links` / `heat_booster_rechts`.

## Ingester rules

- Gateway: use payload `ts`; if missing, use receive time; if more than 5 min in the future, reject the message.
- Heat Booster: stamped with receive time (no timestamp in the firmware payload).
- Retained messages (any source) are skipped.
- Unknown topics, devices (error for unknown Gateway device), and unknown Heat Booster fields are ignored.
- Malformed JSON, `metrics` not an object, or a non-numeric value: error, no partial readings.
- Empty `metrics`: zero readings, no error.
- Rows are idempotent on `(device, metric, time)`.

## Metrics per device

### `heat_pump` (10 s)

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `flow_temp` | °C | Flow Temperature, Heat Pump outlet | TBD (step 9) |
| `return_temp` | °C | Return Temperature, Heat Pump inlet | TBD (step 9) |
| `outdoor_temp` | °C | Outdoor Temperature | TBD (step 9) |
| `room_temp` | °C | Room Temperature (Heat Pump room sensor) | TBD (step 9) |
| `dhw_temp` | °C | DHW tank temperature | TBD (step 9) |
| `target_flow_temp` | °C | Target Flow Temperature from the Heating Curve | TBD (step 9) |
| `flow_rate` | TBD (see step 9 questions) | Flow Rate through the Heat Pump | TBD (step 9), old `lg_current_flow_rate` |
| `curve_shift` | K | Curve Shift (circuit 1) | TBD (step 9) |
| `compressor_on` | 0/1 | Compressor running (raw flag) | TBD (step 9) |
| `defrost_active` | 0/1 | Defrost in progress (raw flag) | TBD (step 9) |
| `dhw_active` | 0/1 | Heating DHW, e.g. 3-way valve (raw flag) | TBD (step 9) |
| `water_pump_on` | 0/1 | Circulation pump running (raw flag) | TBD (step 9) |
| `error_code` | integer | Heat Pump error code, 0 = none | TBD (step 9) |

### `electricity_meter` (10 s)

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `energy_kwh` | kWh | Cumulative counter, Heat Pump electricity consumption | TBD (step 9) |
| `power_w` | W | Instantaneous power | TBD (step 9) |

### `heat_meter` (60 s, battery Kamstrup Multical)

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `energy_kwh` | kWh | Cumulative thermal energy counter, Wh resolution (needed for per-mode COP, ADR-0003) | TBD (step 9) |
| `power_w` | W | Instantaneous thermal power | TBD (step 9) |
| `flow_temp` | °C | Flow Temperature | TBD (step 9) |
| `return_temp` | °C | Return Temperature | TBD (step 9) |
| `flow_rate` | L/h | Flow Rate | TBD (step 9) |
| `volume_m3` | m³ | Cumulative volume counter | TBD (step 9) |

### `heat_booster_links`, `heat_booster_rechts` (~10 s)

| Metric | Unit | Description | Firmware field |
| --- | --- | --- | --- |
| `flow_temp` | °C | Flow Temperature at the radiator inlet | `temp-inlet` |
| `return_temp` | °C | Return Temperature at the radiator outlet | `temp-outlet` |
| `ambient_temp` | °C | Booster Ambient Temperature | `temp-ambient` |
| `fan_speed` | % (observed 25, 30) | Fan speed | `fan-speed` |
| `fan_boost` | 0/1 | Fan boost mode | `fan-boostmode` |
| `rssi` | dBm | WiFi signal strength | `rssi` |

Dropped: `temp-delta-io` (Delta T, derivable from flow/return). All other firmware fields are ignored.

### Observed on the live Broker (2026-11-06, `192.168.1.40`)

Both boosters, firmware v2.9.1, publish all of: `ssid`, `interval` (10), `firmware`, `runtime`, `wifi-reconnects`, `mqtt-reconnects`, `fan-control-state` (text, `manual`), `fan-controlmode`, `fan-speed`, `fan-boostmode`, `fan-enabled`, `temp-inlet`, `temp-outlet`, `temp-delta-io`, `temp-inlet-rate`, `temp-ambient`, `ambientcontrol-enable`, `ambientcontrol-temp`, `ambientcontrol-temp-trim`, `demand-trim`, `rssi`, `ip`. Value formats: plain ASCII numbers, 2 decimals for temperatures (`40.29`), integers for `fan-speed` (`25`), `fan-boostmode` (`0`, so 0/1 integer), `rssi` (`-76`). Text fields (`ssid`, `firmware`, `ip`, `fan-control-state`) are non-numeric and must stay out of the allowlist. Topic casing differs: `heatbooster-links` vs `Heatbooster-rechts`. A first burst on subscribe (a full set of fields arriving instantly) suggests the topics are also retained; hence retained messages are skipped and only live ones stored.

## Example

```sh
docker run --rm eclipse-mosquitto:2 mosquitto_pub -h 192.168.1.40 -q 1 \
  -t heatpump-monitor/gateway/heat_pump \
  -m '{"ts":"2026-11-06T12:00:00Z","metrics":{"flow_temp":35.2,"compressor_on":1}}'
```

## Step 9 open questions

- LG registers actually read, incl. status flags (compressor, defrost, 3-way valve/DHW, water pump); fill the `heat_pump` Source column.
- Electricity Meter model and registers.
- Multical M-Bus fields used for each `heat_meter` metric; allowed read rate for the battery.
- `flow_rate` unit of `lg_current_flow_rate` (1.43: L/min or m³/h?); check against Multical flow.
- Which register `g_target_temp_circuit1` is (publishes 48 vs 39 for target temp): keep under a canonical name or drop.
- Does Home Assistant subscribe to `lg_heatpump/*`? If so add HA MQTT sensors on the new topics.
- Pi clock/NTP (`timedatectl`).
