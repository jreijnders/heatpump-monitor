// Real probe reads from the Gateway, 2026-11-06 ~16:29 UTC
// (Heat Pump unit 1, SDM220 unit 10, 9600 8N1 RTU; Kamstrup M-Bus secondary address 828569592D2C400C).
"use strict";

module.exports = {
  // FC4 input regs 0..13 (14..15 and 25+ answer Modbus exception 2)
  input0: [0, 2, 429, 450, 64887, 550, 3000, 195, 196, 64887, 64890, 0, 70, 0],
  // FC4 input regs 16..24
  input16: [439, 12000, 34, 690, 55, 55, 2827, 778, 22],
  // FC2 discrete inputs 0..15 (the flow reads 0..13)
  discrete0: [true, true, false, true, false, false, false, true, true, false, false, false, false, false, false, false].slice(0, 14),
  // FC2 replies are padded to whole bytes: quantity 14 returned 16 booleans live
  discrete16: [true, true, false, true, false, false, false, true, true, false, false, false, false, false, false, false],
  // FC3 holding regs 0..9
  holding: [3, 0, 420, 0, 0, 0, 0, 0, 550, 0],
  // SDM220 FC4 342..343: total active energy kWh, float32 BE word order
  sdmTotalEnergyWords: [18033, 10142],
  // Live MQTT at the same time (old flow): import_active_energy 15451.75, active_power 1.2 kW, voltage 222.6
  // Kamstrup DataRecord (id, Function, StorageNumber, Unit, Value)
  mbusRecords: [
    [0, "Instantaneous value", 0, "Energy (kWh)", 44584],
    [1, "Instantaneous value", 0, "Volume (1e-2  m^3)", 1448216],
    [2, "Instantaneous value", 0, "Manufacturer specific", 648309],
    [3, "Instantaneous value", 0, "Manufacturer specific", 611893],
    [4, "Instantaneous value", 0, "On time (hours)", 33620],
    [5, "Value during error state", 0, "On time (hours)", 4369],
    [6, "Instantaneous value", 0, "Flow temperature (1e-2 deg C)", 4703],
    [7, "Instantaneous value", 0, "Return temperature (1e-2 deg C)", 4472],
    [8, "Instantaneous value", 0, "Temperature Difference (1e-2  deg C)", 231],
    [9, "Instantaneous value", 0, "Power (100 W)", 32],
    [10, "Maximum value", 0, "Power (100 W)", 116],
    [11, "Instantaneous value", 0, "Volume flow (m m^3/h)", 1216],
    [12, "Maximum value", 0, "Volume flow (m m^3/h)", 1633],
    [13, "Instantaneous value", 0, "Manufacturer specific", 0],
    [14, "Instantaneous value", 1, "Energy (kWh)", 33784], // StorageNumber 1: stored/billing-date value, ignored
  ].map(([id, Function, StorageNumber, Unit, Value]) => ({
    id, Function, StorageNumber, Unit, Value, Timestamp: "2026-11-06T16:29:07Z",
  })),
};
