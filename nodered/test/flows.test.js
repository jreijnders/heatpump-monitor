"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { loadFlows } = require("./harness");

const flows = loadFlows();
const byId = Object.fromEntries(flows.map((n) => [n.id, n]));
const REMOVED = ["d3d97e939de5969c", "dfb50251.88088", "f935c11c.41462", "e55023e1.1202d", "4009050ee9c6c1f7",
  "7233c9f7b6345c98", "bc9a0d815bdd7702", "35b24b20.e53a34"];
const KEPT_CONTROL = ["de04bd503e556943", "8d1f64a58784c848", "b940218caad110b9", "d4089c392ecefc3b", "cded0a8175f212f0"];

test("every mqtt out: QoS 1, retain false, topic heatpump-monitor/gateway/<device>", () => {
  const outs = flows.filter((n) => n.type === "mqtt out");
  assert.equal(outs.length, 3);
  assert.deepEqual(outs.map((n) => n.topic).sort(), ["heatpump-monitor/gateway/electricity_meter", "heatpump-monitor/gateway/heat_meter", "heatpump-monitor/gateway/heat_pump"]);
  for (const n of outs) { assert.equal(n.qos, "1"); assert.equal(n.retain, "false"); assert.equal(n.broker, "8ec630e3667c0136"); }
});

test("each Build node feeds the mqtt out of its own device", () => {
  for (const device of ["heat_pump", "electricity_meter", "heat_meter"]) {
    const build = flows.find((n) => n.name === "Build " + device);
    const targets = build.wires[0].map((id) => byId[id]).filter((n) => n.type === "mqtt out");
    assert.equal(targets.length, 1);
    assert.equal(targets[0].topic, "heatpump-monitor/gateway/" + device);
  }
});

test("removed nodes are gone and nothing references them", () => {
  const text = JSON.stringify(loadFlows());
  for (const id of REMOVED) { assert.ok(!byId[id], id); assert.ok(!text.includes(id), `reference to ${id}`); }
});

test("no dangling wires or config references", () => {
  for (const n of flows) {
    for (const id of (n.wires || []).flat()) assert.ok(byId[id], `${n.id} wires to missing ${id}`);
    for (const k of ["server", "broker", "client", "z"]) if (n[k] && n.type !== "mqtt-broker") assert.ok(byId[n[k]], `${n.id}.${k} -> missing ${n[k]}`);
  }
});

test("no stale per-value / Home Assistant publishing", () => {
  const text = JSON.stringify(loadFlows());
  for (const s of ["lg_heatpump", "hp_electric", "hp_production", "homeassistant"]) assert.ok(!text.includes(s), s);
  assert.deepEqual(flows.filter((n) => n.type === "tab").map((n) => n.label), ["LG Heatpump"]);
});

test("serial ports use stable by-id paths", () => {
  assert.equal(byId["a6c2b30e7d452d55"].serialPort, "/dev/serial/by-id/usb-1a86_USB2.0-Serial-if00-port0");
  assert.equal(byId["b8af9f24.8bce"].serialPort, "/dev/serial/by-id/usb-FTDI_FT232R_USB_UART_AQ02BOCQ-if00-port0");
  for (const n of flows.filter((n) => n.serialPort)) assert.match(n.serialPort, /^\/dev\/serial\/by-id\//);
});

test("flex-sequencers: output 2 unwired (it carries the RAW response on SUCCESS, not errors)", () => {
  // node-red-contrib-modbus: output 1 = values, output 2 = raw response. Failures arrive on output 1 as an
  // empty payload (emptyMsgOnFail), which Stash/Build reject and use to drop the cycle.
  const seqs = flows.filter((n) => n.type === "modbus-flex-sequencer");
  assert.equal(seqs.length, 5);
  for (const s of seqs) {
    assert.equal(s.emptyMsgOnFail, true);
    assert.deepEqual(s.wires[1], []);
    assert.equal(s.server, "a6c2b30e7d452d55");
  }
  assert.equal(flows.filter((n) => /^Abort /.test(n.name || "")).length, 0);
  const hp = seqs.filter((s) => s.sequences[0].unitid === "1").map((s) => `${s.sequences[0].fc}:${s.sequences[0].address}+${s.sequences[0].quantity}`);
  assert.deepEqual(hp, ["FC4:0+14", "FC4:16+9", "FC2:0+14", "FC3:0+10"]);
});

test("no function node relies on custom msg properties surviving a sequencer (msg.ts / msg.lg)", () => {
  for (const n of flows.filter((n) => n.type === "function")) assert.doesNotMatch(n.func, /msg\.(ts|lg)\b/, n.name);
});

test("Build functions stay short and flat (maintainability)", () => {
  for (const n of flows.filter((n) => /^Build /.test(n.name || ""))) assert.ok(n.func.split("\n").length <= 50, n.name);
});

test("polling intervals: 10 s / 10 s offset / 60 s; kept control injects unchanged", () => {
  const rep = (name) => flows.find((n) => n.type === "inject" && n.name.startsWith(name));
  assert.equal(rep("heat_pump").repeat, "10");
  assert.equal(rep("electricity_meter").repeat, "10");
  assert.notEqual(rep("heat_pump").onceDelay, rep("electricity_meter").onceDelay);
  assert.equal(rep("heat_meter").repeat, "60");
  assert.equal(rep("heat_meter").topic, "getDevice");
  assert.deepEqual(rep("heat_meter").payload, '{"address": "828569592D2C400C"}');
  for (const id of KEPT_CONTROL) assert.ok(byId[id], id);
  assert.equal(byId["de04bd503e556943"].repeat, "1");
  assert.equal(byId["b940218caad110b9"].repeat, "10");
  assert.deepEqual(byId["b940218caad110b9"].wires, [["d4089c392ecefc3b"]]);
});
