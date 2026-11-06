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

Gateway flow: `nodered/flows.json` (function code unit-tested in `nodered/test/`). Invalid values are omitted, never `null`. If any read of a heat_pump cycle fails, that cycle publishes nothing (no partial snapshots). Cycles never overlap: an inject is skipped while the previous cycle (heat_pump, electricity_meter) is still in flight (< 25 s).

### `heat_pump` (10 s)

LG Therma V, Modbus RTU unit 1 (9600 8N1, RS485). Registers per `docs/research/lg-therma-v-modbus.md`. Temperatures are int16 (tenths of °C); values <= -60 °C (raw 64887 = -64.9, "sensor not connected") are omitted.

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `error_code` | integer | Heat Pump error code, 0 = none | FC4 input 0 |
| `odu_cycle` | 0/1/2 | Outdoor unit cycle: 0 standby, 1 cooling, 2 heating | FC4 input 1 |
| `return_temp` | °C | Return Temperature, Heat Pump water inlet | FC4 input 2, /10 |
| `flow_temp` | °C | Flow Temperature, Heat Pump water outlet | FC4 input 3, /10 |
| `dhw_temp` | °C | DHW tank temperature | FC4 input 5, /10 |
| `room_temp` | °C | Room Temperature (Heat Pump room sensor) | FC4 input 7, /10 |
| `flow_rate` | L/h | Flow Rate through the Heat Pump | FC4 input 8, raw 0.1 L/min, x6 |
| `energy_state` | enum | Energy state input (raw enum): 0 unused, 1 forced off, 2 normal, 3 on-recommend, 4 on-command, 5 on-command step 2, 6 on-recommend step 1, 7 energy saving, 8 super energy saving (per `docs/research/lg-therma-v-modbus.md`, listed for the holding-9 write; input 11 reads 0 here) | FC4 input 11 |
| `outdoor_temp` | °C | Outdoor Temperature | FC4 input 12, /10 |
| `liquid_temp` | °C | Refrigerant liquid pipe temperature | FC4 input 16, /10 |
| `suction_temp` | °C | Compressor suction temperature | FC4 input 18, /10 |
| `discharge_temp` | °C | Compressor discharge temperature | FC4 input 19, /10 |
| `evaporator_in_temp` | °C | Evaporator inlet temperature | FC4 input 20, /10 |
| `evaporator_out_temp` | °C | Evaporator outlet temperature | FC4 input 21, /10 |
| `high_pressure` | bar | Refrigerant high pressure | FC4 input 22, /100 |
| `low_pressure` | bar | Refrigerant low pressure | FC4 input 23, /100 |
| `compressor_hz` | Hz | Compressor frequency; raw value is Hz (observed 22 at low modulation, 0 when off). The rpm interpretation would be raw x60 | FC4 input 24 |
| `water_pump_on` | 0/1 | Circulation pump running (raw flag) | FC2 discrete 1 |
| `compressor_on` | 0/1 | Compressor running (raw flag) | FC2 discrete 3 |
| `defrost_active` | 0/1 | Defrost in progress (raw flag) | FC2 discrete 4 |
| `dhw_active` | 0/1 | Heating DHW, 3-way valve (raw flag) | FC2 discrete 5 |
| `dhw_disinfection_active` | 0/1 | DHW disinfection running (raw flag) | FC2 discrete 6 |
| `silent_mode` | 0/1 | Silent mode active (raw flag) | FC2 discrete 7 |
| `backup_heater_1_on` | 0/1 | Backup Heater stage 1 on (raw flag) | FC2 discrete 10 |
| `backup_heater_2_on` | 0/1 | Backup Heater stage 2 on (raw flag) | FC2 discrete 11 |
| `dhw_heater_on` | 0/1 | DHW heater on (raw flag) | FC2 discrete 12 |
| `error_active` | 0/1 | Error present (raw flag) | FC2 discrete 13 |
| `mode_setting` | 0/3/4 | Configured mode: 0 cool, 3 auto, 4 heat | FC3 holding 0 |
| `control_method` | integer | Control method setting | FC3 holding 1 |
| `target_flow_temp` | °C | Target Flow Temperature from the Heating Curve | FC3 holding 2, /10 |
| `curve_shift` | K | Curve Shift (circuit 1), int16 | FC3 holding 4 |
| `dhw_target_temp` | °C | DHW target temperature | FC3 holding 8, /10 |

Deliberately not published: backup heater temperature (input 4, garbage), solar (input 6), circuit-2 values (inputs 9, 10, holding 5-7), input 13 (water pressure: reads 0, unsupported on this unit), discrete 0 and 8 (ambiguous).

### `electricity_meter` (10 s, offset 4 s from `heat_pump`)

Eastron SDM220, Modbus RTU unit 10 on the same RS485 bus as the Heat Pump. One FC4 read of input regs 0..79; float32, big-endian word order (high word first).

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `energy_kwh` | kWh | Cumulative counter, Heat Pump electricity consumption (import active energy) | FC4 input 72-73, float32, 3 decimals |
| `power_w` | W | Instantaneous active power | FC4 input 12-13, float32, 2 decimals |
| `voltage` | V | Line voltage | FC4 input 0-1, float32, 2 decimals |
| `current` | A | Line current | FC4 input 6-7, float32, 3 decimals |
| `power_factor` | ratio | Power factor | FC4 input 30-31, float32, 3 decimals |

### `heat_meter` (60 s, battery Kamstrup Multical)

M-Bus (2400 baud), secondary address `828569592D2C400C`, `getDevice` read. Records are matched by `StorageNumber` 0, Function `Instantaneous value` and the Unit text, never by position. Stored values (`StorageNumber` 1+, billing date) and max values are ignored. Scaling is derived from the Unit text (e.g. `1e-2` multiplier, `100` multiplier, `m` milli prefix, Wh/kWh/MWh, W/kW).

| Metric | Unit | Description | Source |
| --- | --- | --- | --- |
| `energy_kwh` | kWh | Cumulative thermal energy counter, **1 kWh resolution** (calibration anchor for the flow-times-delta-T COP estimate, `docs/adr/0003-cop-from-flow-delta-t-calibrated-to-counter.md`) | Unit `Energy (kWh)` |
| `power_w` | W | Instantaneous thermal power | Unit `Power (100 W)`, x100 |
| `flow_temp` | °C | Flow Temperature | Unit `Flow temperature (1e-2 deg C)`, x0.01 |
| `return_temp` | °C | Return Temperature | Unit `Return temperature (1e-2 deg C)`, x0.01 |
| `flow_rate` | L/h | Flow Rate | Unit `Volume flow (m m^3/h)` = 1e-3 m³/h = L/h, x1 |
| `volume_m3` | m³ | Cumulative volume counter | Unit `Volume (1e-2  m^3)` (double space), x0.01 |

`ts` for `heat_meter`: always the read time (`Date.now()` when the reply reaches the Gateway function). The meter's own record `Timestamp` is ignored: the meter clock may be off and the SQL relies on real sample spacing.

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

## Step 9 findings (resolved)

- Registers/records per device: see the Source columns above. Probe data 2026-11-06 in `nodered/test/fixtures.js`.
- `flow_rate`: L/h on both devices (LG raw is 0.1 L/min, x6; Kamstrup `m m^3/h` = L/h). Probe: 1176 vs 1216 L/h, consistent.
- Water pressure (input 13) is unsupported on this unit (reads 0): not published.
- Heat Meter energy has 1 kWh resolution (`Energy (kWh)`); the counter is the calibration anchor for the flow-times-delta-T COP, see `docs/adr/0003-cop-from-flow-delta-t-calibrated-to-counter.md`.
- `g_target_temp_circuit1` was a stale retained-only topic with no publisher: dropped. Target Flow Temperature is `target_flow_temp` (holding 2).
- Home Assistant has been decommissioned: no `lg_heatpump/*`, `hp_electric/*`, `hp_production/*` or discovery topics are published any more. Stale retained ones on the Broker can be cleared.
- Electricity Meter is an Eastron SDM220, Modbus unit 10, same RS485 bus as the Heat Pump.
- Kamstrup secondary address `828569592D2C400C`; read every 60 s.
- Serial adapters use stable `/dev/serial/by-id/` paths (Modbus RS485: CH340 `usb-1a86_USB2.0-Serial-if00-port0`; M-Bus: FTDI `usb-FTDI_FT232R_USB_UART_AQ02BOCQ-if00-port0`), since ttyUSB0/1 can swap on reboot.
- Still open: Pi clock/NTP (`timedatectl`); Kamstrup allowed read rate (60 s assumed OK).

## Existing Gateway behaviour outside this project

The flow also writes to the Heat Pump: coil 2 "silent mode" every 1 s and holding register 9 "energy state" = 0 every 10 s. Heat pump control, kept unchanged, not part of the monitoring contract.
