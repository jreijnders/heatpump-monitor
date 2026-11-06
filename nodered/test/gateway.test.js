"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fx = require("./fixtures");
const { runFunction } = require("./harness");

const NOW = Date.parse("2026-11-06T16:29:10.000Z");
const RFC3339_MS = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/;

// Independent float32 decode (no Buffer) to cross-check the node's decoding.
const f32FromWords = (hi, lo) => {
  const bits = ((hi << 16) | lo) >>> 0;
  const sign = bits >>> 31 ? -1 : 1, exp = (bits >>> 23) & 0xff, man = bits & 0x7fffff;
  return sign * (1 + man / 0x800000) * Math.pow(2, exp - 127);
};
const encodeF32 = (x) => { const b = Buffer.alloc(4); b.writeFloatBE(x, 0); return [b.readUInt16BE(0), b.readUInt16BE(2)]; };

// ---------- heat_pump ----------
// Seeds flow context like Start + Stash would; the Build msg carries ONLY the holding block (sequencer strips the rest).
function runHeatPump(over = {}) {
  const lg = { input0: fx.input0, input16: fx.input16, discrete0: fx.discrete0, ...(over.lg || {}) };
  const flowCtx = { hp_cycle: "cycle" in over ? over.cycle : { ts: NOW - 500, lg } };
  const r = runFunction("Build heat_pump", { payload: "holding" in over ? over.holding : fx.holding }, { flowCtx });
  r.flowCtx = flowCtx;
  return r;
}
const hpMetrics = (over) => JSON.parse(runHeatPump(over).result.payload).metrics;

test("heat_pump: Start node resets an expired snapshot and stamps read start", () => {
  const flowCtx = { hp_cycle: { ts: NOW - 26000, lg: { input0: [1] } } };
  const { result } = runFunction("Start heat_pump read", { payload: "x" }, { flowCtx });
  assert.deepEqual(JSON.parse(JSON.stringify(flowCtx.hp_cycle)), { ts: NOW, lg: {} });
  assert.equal(result.payload, true);
});

test("heat_pump: probe snapshot -> exact metrics", () => {
  assert.deepEqual(hpMetrics(), {
    error_code: 0, odu_cycle: 2, return_temp: 42.9, flow_temp: 45, dhw_temp: 55, room_temp: 19.5,
    flow_rate: 1176, energy_state: 0, outdoor_temp: 7,
    liquid_temp: 43.9, suction_temp: 3.4, discharge_temp: 69, evaporator_in_temp: 5.5, evaporator_out_temp: 5.5,
    high_pressure: 28.27, low_pressure: 7.78, compressor_hz: 22,
    water_pump_on: 1, compressor_on: 1, defrost_active: 0, dhw_active: 0, dhw_disinfection_active: 0, silent_mode: 1,
    backup_heater_1_on: 0, backup_heater_2_on: 0, dhw_heater_on: 0, error_active: 0,
    mode_setting: 3, control_method: 0, target_flow_temp: 42, curve_shift: 0, dhw_target_temp: 55,
  });
});

test("heat_pump: unsupported/ambiguous registers never published", () => {
  const m = hpMetrics();
  for (const k of Object.keys(m)) assert.doesNotMatch(k, /backup_heater_temp|solar|circuit2|water_pressure|pressure_water/);
  assert.equal(Object.keys(m).length, 32);
});

test("heat_pump: sentinel temperatures omitted (raw 64887 = -64.9)", () => {
  const input0 = fx.input0.slice(); input0[3] = 64887; input0[12] = 64890;
  const m = hpMetrics({ lg: { input0 } });
  assert.ok(!("flow_temp" in m));
  assert.ok(!("outdoor_temp" in m));
  assert.equal(m.return_temp, 42.9);
});

test("heat_pump: negative outdoor temperature decoded as int16", () => {
  const input0 = fx.input0.slice(); input0[12] = 65536 - 35; // -3.5 degC
  assert.equal(hpMetrics({ lg: { input0 } }).outdoor_temp, -3.5);
});

test("heat_pump: negative curve_shift decoded as int16", () => {
  assert.equal(hpMetrics({ holding: [3, 0, 420, 0, 65535, 0, 0, 0, 550, 0] }).curve_shift, -1);
});

test("heat_pump: discrete booleans become 0/1 numbers", () => {
  const discrete0 = new Array(14).fill(false); discrete0[12] = true; discrete0[13] = true;
  const m = hpMetrics({ lg: { discrete0 } });
  assert.equal(m.dhw_heater_on, 1); assert.equal(m.error_active, 1); assert.equal(m.compressor_on, 0);
});

test("heat_pump: payload is JSON string, RFC3339 UTC ms ts from read start, numbers only", () => {
  const { result } = runHeatPump();
  assert.equal(typeof result.payload, "string");
  const p = JSON.parse(result.payload);
  assert.deepEqual(Object.keys(p), ["ts", "metrics"]);
  assert.match(p.ts, RFC3339_MS);
  assert.equal(p.ts, new Date(NOW - 500).toISOString());
  for (const v of Object.values(p.metrics)) assert.equal(typeof v, "number");
  assert.ok(!result.payload.includes("null"));
});

test("heat_pump: missing or short block publishes nothing", () => {
  for (const over of [
    { lg: { input16: undefined } }, { lg: { input0: fx.input0.slice(0, 13) } },
    { lg: { discrete0: [] } }, { holding: [3, 0] }, { holding: "" },
  ]) {
    const r = runHeatPump(over);
    assert.equal(r.result, null);
    assert.equal(r.warns.length, 1);
  }
  assert.equal(runHeatPump({ cycle: undefined }).result, null); // no snapshot at all
  assert.equal(runHeatPump({ cycle: { lg: {} } }).result, null); // no ts
});

test("heat_pump: stale snapshot (> 25 s) publishes nothing; fresh edge publishes; snapshot consumed", () => {
  const lg = { input0: fx.input0, input16: fx.input16, discrete0: fx.discrete0 };
  assert.equal(runHeatPump({ cycle: { ts: NOW - 25001, lg } }).result, null);
  assert.equal(runHeatPump({ cycle: { ts: NOW + 1000, lg } }).result, null); // future
  const ok = runHeatPump({ cycle: { ts: NOW - 25000, lg } });
  assert.ok(ok.result);
  assert.equal(ok.flowCtx.hp_cycle, undefined); // cleared: a second Build without Start publishes nothing
  assert.equal(runFunction("Build heat_pump", { payload: fx.holding }, { flowCtx: ok.flowCtx }).result, null);
  assert.equal(runHeatPump({ cycle: { ts: NOW - 25001, lg } }).flowCtx.hp_cycle, undefined); // stale also cleared
});

test("heat_pump: stash nodes write flow context, reset payload, reject bad payloads (failed read)", () => {
  const cases = [["Stash input0", "input0", fx.input0], ["Stash input16", "input16", fx.input16], ["Stash discrete0", "discrete0", fx.discrete0]];
  for (const [name, key, data] of cases) {
    const flowCtx = { hp_cycle: { ts: NOW, lg: {} } };
    const ok = runFunction(name, { payload: data }, { flowCtx }).result;
    assert.deepEqual(flowCtx.hp_cycle.lg[key].slice(), data);
    assert.equal(ok.payload, true);
    assert.equal(runFunction(name, { payload: "" }, { flowCtx }).result, null); // emptyMsgOnFail style
    assert.equal(runFunction(name, { payload: data.slice(1) }, { flowCtx }).result, null);
    assert.equal(runFunction(name, { payload: data }, { flowCtx: {} }).result, null); // no Start snapshot
  }
});

test("heat_pump: FC2 padded to 16 booleans is accepted (first 14 used); registers must be exact", () => {
  const flowCtx = { hp_cycle: { ts: NOW, lg: {} } };
  assert.ok(runFunction("Stash discrete0", { payload: fx.discrete16 }, { flowCtx }).result);
  assert.equal(flowCtx.hp_cycle.lg.discrete0.length, 14);
  assert.equal(runFunction("Stash input0", { payload: fx.input0.concat([0, 0]) }, { flowCtx }).result, null);
  assert.equal(runFunction("Stash input16", { payload: fx.input16.concat([0]) }, { flowCtx }).result, null);
});

test("heat_pump: overlapping Start while a cycle is in flight is skipped and leaves the snapshot alone", () => {
  const flowCtx = {};
  runFunction("Start heat_pump read", { payload: true }, { flowCtx }); // cycle A starts at NOW
  runFunction("Stash input0", { payload: fx.input0 }, { flowCtx });
  const later = NOW + 10000; // next inject while A is still running
  const r = runFunction("Start heat_pump read", { payload: true }, { flowCtx, now: later });
  assert.equal(r.result, null);
  assert.equal(r.warns.length, 1);
  assert.equal(flowCtx.hp_cycle.ts, NOW);
  assert.equal(flowCtx.hp_cycle.lg.input0.length, 14);
  // A finishes late (still inside its window) and publishes with its own ts
  runFunction("Stash input16", { payload: fx.input16 }, { flowCtx, now: later });
  runFunction("Stash discrete0", { payload: fx.discrete16 }, { flowCtx, now: later });
  const done = runFunction("Build heat_pump", { payload: fx.holding }, { flowCtx, now: later }).result;
  assert.equal(JSON.parse(done.payload).ts, new Date(NOW).toISOString());
  // once consumed, the next inject starts a new cycle
  assert.ok(runFunction("Start heat_pump read", { payload: true }, { flowCtx, now: later + 10000 }).result);
  assert.equal(flowCtx.hp_cycle.ts, later + 10000);
});

test("heat_pump: Start after an expired in-flight snapshot starts fresh", () => {
  const flowCtx = { hp_cycle: { ts: NOW - 25000, lg: { input0: [9] } } };
  assert.ok(runFunction("Start heat_pump read", { payload: true }, { flowCtx }).result);
  assert.equal(flowCtx.hp_cycle.ts, NOW);
});

test("heat_pump: a bad block clears the snapshot (cycle dropped, next inject not blocked)", () => {
  const flowCtx = { hp_cycle: { ts: NOW, lg: {} } };
  assert.equal(runFunction("Stash input0", { payload: "" }, { flowCtx }).result, null);
  assert.equal(flowCtx.hp_cycle, undefined);
});

function runCycle(flowCtx, { discrete = fx.discrete16, failAt = null } = {}) {
  // Each sequencer output msg carries only payload: model the stripping.
  runFunction("Start heat_pump read", { payload: true }, { flowCtx });
  const steps = [["Stash input0", fx.input0], ["Stash input16", fx.input16], ["Stash discrete0", discrete]];
  for (const [name, data] of steps) {
    if (failAt === name) return runFunction(name, { payload: "" }, { flowCtx }).result; // failed read: emptyMsgOnFail -> empty payload
    if (!runFunction(name, { payload: data }, { flowCtx }).result) return null;
  }
  if (failAt === "Build") return null;
  return runFunction("Build heat_pump", { payload: fx.holding }, { flowCtx }).result;
}

test("heat_pump: end-to-end chain Start -> Stash x3 -> Build with 16-bool discrete block", () => {
  const r = runCycle({});
  const p = JSON.parse(r.payload);
  assert.equal(p.ts, new Date(NOW).toISOString());
  assert.deepEqual(p.metrics, hpMetrics());
  assert.equal(p.metrics.compressor_on, 1);
  assert.equal(p.metrics.silent_mode, 1);
});

test("heat_pump: failed middle read publishes nothing and the next cycle is clean", () => {
  const flowCtx = {};
  assert.equal(runCycle(flowCtx, { failAt: "Stash discrete0" }), null);
  // the empty payload of the failed read dropped the snapshot
  assert.equal(flowCtx.hp_cycle, undefined);
  // a Build that somehow runs anyway publishes nothing
  assert.equal(runFunction("Build heat_pump", { payload: fx.holding }, { flowCtx }).result, null);
  // next cycle starts fresh and is complete
  const r = runCycle(flowCtx);
  assert.deepEqual(JSON.parse(r.payload).metrics, hpMetrics());
  // a read that never answers at all: half-filled snapshot blocks Start until it expires, then recovers
  const ctx2 = {};
  runFunction("Start heat_pump read", { payload: true }, { flowCtx: ctx2 });
  runFunction("Stash input0", { payload: fx.input0 }, { flowCtx: ctx2 });
  assert.equal(runFunction("Start heat_pump read", { payload: true }, { flowCtx: ctx2, now: NOW + 10000 }).result, null);
  assert.ok(runFunction("Start heat_pump read", { payload: true }, { flowCtx: ctx2, now: NOW + 25001 }).result);
});

test("heat_pump: leftovers of a failed cycle never leak into the next (Start resets lg)", () => {
  const flowCtx = { hp_cycle: { ts: NOW - 30000, lg: { input0: [9], discrete0: [true] } } };
  runFunction("Start heat_pump read", { payload: true }, { flowCtx });
  assert.deepEqual(JSON.parse(JSON.stringify(flowCtx.hp_cycle.lg)), {});
});

// ---------- electricity_meter ----------
function sdmRegs(over = {}) {
  const regs = new Array(80).fill(0);
  const put = (r, x) => { const [hi, lo] = encodeF32(x); regs[r] = hi; regs[r + 1] = lo; };
  put(0, over.voltage ?? 222.6); put(6, over.current ?? 5.4); put(12, over.power ?? 1200); put(30, over.pf ?? 0.97);
  if (over.words) { regs[72] = over.words[0]; regs[73] = over.words[1]; } else put(72, over.energy ?? 15451.75);
  return regs;
}
// Sequencer output has no custom props: read start time comes only from flow context (set by Start).
const runEm = (regs, t0 = NOW - 200) => runFunction("Build electricity_meter", { payload: regs }, { flowCtx: { em_ts: t0 } });

test("electricity_meter: float32 BE decode (no byte overlap): 222.6 V and friends", () => {
  const p = JSON.parse(runEm(sdmRegs()).result.payload);
  assert.equal(p.metrics.voltage, 222.6);
  assert.equal(p.metrics.current, 5.4);
  assert.equal(p.metrics.power_w, 1200);
  assert.equal(p.metrics.power_factor, 0.97);
  assert.equal(p.metrics.energy_kwh, 15451.75);
  assert.match(p.ts, RFC3339_MS);
  assert.equal(p.ts, new Date(NOW - 200).toISOString());
});

test("electricity_meter: Start stores read start; stale/absent start publishes nothing; consumed", () => {
  const flowCtx = {};
  runFunction("Start electricity_meter read", { payload: "x" }, { flowCtx });
  assert.equal(flowCtx.em_ts, NOW);
  const ok = runFunction("Build electricity_meter", { payload: sdmRegs() }, { flowCtx });
  assert.equal(JSON.parse(ok.result.payload).ts, new Date(NOW).toISOString());
  assert.equal(flowCtx.em_ts, undefined);
  assert.equal(runFunction("Build electricity_meter", { payload: sdmRegs() }, { flowCtx }).result, null);
  assert.equal(runEm(sdmRegs(), NOW - 25001).result, null);
  assert.equal(runEm(sdmRegs(), null).result, null);
});

test("electricity_meter: overlapping Start is skipped; failed read clears; expired start restarts", () => {
  const flowCtx = { em_ts: NOW };
  const r = runFunction("Start electricity_meter read", { payload: true }, { flowCtx, now: NOW + 10000 });
  assert.equal(r.result, null);
  assert.equal(flowCtx.em_ts, NOW);
  assert.equal(runFunction("Build electricity_meter", { payload: "" }, { flowCtx, now: NOW + 1000 }).result, null); // failed read
  assert.equal(flowCtx.em_ts, undefined);
  assert.ok(runFunction("Start electricity_meter read", { payload: true }, { flowCtx, now: NOW + 10000 }).result);
  const old = { em_ts: NOW - 25000 };
  assert.ok(runFunction("Start electricity_meter read", { payload: true }, { flowCtx: old }).result);
  assert.equal(old.em_ts, NOW);
});

test("electricity_meter: probe words [18033,10142] ~ 15433.x kWh, exact", () => {
  const expected = Math.round(f32FromWords(...fx.sdmTotalEnergyWords) * 1000) / 1000;
  assert.ok(expected > 15433 && expected < 15434, String(expected));
  const p = JSON.parse(runEm(sdmRegs({ words: fx.sdmTotalEnergyWords })).result.payload);
  assert.equal(p.metrics.energy_kwh, expected);
});

test("electricity_meter: NaN registers omitted, short block publishes nothing", () => {
  const regs = sdmRegs(); regs[30] = 0x7fc0; regs[31] = 0; // NaN
  const m = JSON.parse(runEm(regs).result.payload).metrics;
  assert.ok(!("power_factor" in m)); assert.ok("voltage" in m);
  assert.equal(runEm(new Array(10).fill(0)).result, null);
  assert.equal(runEm("").result, null);
  const bad = sdmRegs(); bad[0] = 70000;
  assert.equal(runEm(bad).result, null);
});

// ---------- heat_meter ----------
const records = () => fx.mbusRecords.map((r) => ({ ...r }));
const runHm = (recs, extra = {}) => runFunction("Build heat_meter", { payload: { SlaveInformation: {}, DataRecord: recs }, ...extra });

test("heat_meter: probe records -> exact metrics", () => {
  const p = JSON.parse(runHm(records()).result.payload);
  assert.deepEqual(p.metrics, {
    energy_kwh: 44584, volume_m3: 14482.16, flow_temp: 47.03, return_temp: 44.72, power_w: 3200, flow_rate: 1216,
  });
});

test("heat_meter: shuffled record order and re-numbered ids give the same result", () => {
  const recs = records().reverse().map((r, i) => ({ ...r, id: 100 + i }));
  assert.deepEqual(JSON.parse(runHm(recs).result.payload).metrics, JSON.parse(runHm(records()).result.payload).metrics);
});

test("heat_meter: StorageNumber 1 and non-instantaneous records ignored", () => {
  const recs = records().filter((r) => r.StorageNumber === 0 && r.Function === "Instantaneous value" && !/Energy/.test(r.Unit));
  recs.push({ id: 1, Function: "Instantaneous value", StorageNumber: 1, Unit: "Energy (kWh)", Value: 33784 });
  recs.push({ id: 2, Function: "Maximum value", StorageNumber: 0, Unit: "Power (100 W)", Value: 116 });
  const m = JSON.parse(runHm(recs).result.payload).metrics;
  assert.ok(!("energy_kwh" in m));
  assert.equal(m.power_w, 3200);
});

test("heat_meter: generic unit scaling", () => {
  const rec = (Unit, Value) => ({ id: 0, Function: "Instantaneous value", StorageNumber: 0, Unit, Value });
  const e = (u, v) => JSON.parse(runHm([rec(u, v)]).result.payload).metrics.energy_kwh;
  assert.equal(e("Energy (Wh)", 44584123), 44584.123);
  assert.equal(e("Energy (10 Wh)", 4458412), 44584.12);
  assert.equal(e("Energy (1e-1  kWh)", 445841), 44584.1);
  assert.equal(e("Energy (MWh)", 12), 12000);
  assert.equal(runHm([rec("Energy (GJ)", 5)]).result, null); // unknown unit -> nothing
});

test("heat_meter: ts is always the read time (Date.now()), never the meter Timestamp", () => {
  const ts = (recs) => JSON.parse(runHm(recs).result.payload).ts;
  const at = (T) => records().map((r) => ({ ...r, Timestamp: T }));
  const nowIso = new Date(NOW).toISOString();
  for (const T of ["2026-11-06T16:29:07Z", "2026-11-06T16:29:07", "1970-01-01T00:00:00Z", "2030-01-01T00:00:00Z", "garbage"]) assert.equal(ts(at(T)), nowIso);
  assert.equal(ts(records().map(({ Timestamp, ...r }) => r)), nowIso);
  assert.match(nowIso, RFC3339_MS);
  assert.equal(runFunction("Build heat_meter", { ts: 5, payload: { DataRecord: records() } }).result.payload.includes('"ts":"1970'), false);
});

test("heat_meter: bad input publishes nothing; payload JSON string, numbers only", () => {
  assert.equal(runFunction("Build heat_meter", { payload: "" }).result, null);
  assert.equal(runFunction("Build heat_meter", { payload: { DataRecord: "x" } }).result, null);
  assert.equal(runHm([]).result, null);
  const r = runFunction("Build heat_meter", { payload: JSON.stringify({ DataRecord: records() }) }).result; // string payload tolerated
  assert.equal(typeof r.payload, "string");
  for (const v of Object.values(JSON.parse(r.payload).metrics)) assert.equal(typeof v, "number");
  const nv = records(); nv[0].Value = null; nv[6].Value = "n/a";
  const m = JSON.parse(runHm(nv).result.payload).metrics;
  assert.ok(!("energy_kwh" in m)); assert.ok(!("flow_temp" in m)); assert.equal(m.return_temp, 44.72);
});
