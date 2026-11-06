"use strict";
// Loads function-node code from nodered/flows.json (single source of truth) and runs it in a vm sandbox.
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const FLOWS = path.join(__dirname, "..", "flows.json");
const loadFlows = () => JSON.parse(fs.readFileSync(FLOWS, "utf8"));

function findNode(name, type) {
  const hits = loadFlows().filter((n) => n.name === name && (!type || n.type === type));
  if (hits.length !== 1) throw new Error(`expected exactly one node named "${name}", found ${hits.length}`);
  return hits[0];
}

function makeDate(nowMs) {
  return class FakeDate extends Date {
    constructor(...a) { a.length ? super(...a) : super(nowMs); }
    static now() { return nowMs; }
  };
}

// Runs the function node; returns {result, sent, warns, errors}. result is the return value (msg | null | array).
function runFunction(name, msg, { now = Date.parse("2026-11-06T16:29:10.000Z"), flowCtx = {}, globalCtx = {} } = {}) {
  const n = findNode(name, "function");
  const sent = [], warns = [], errors = [];
  const store = (o) => ({ get: (k) => (o[k] === undefined ? undefined : JSON.parse(JSON.stringify(o[k]))), // clone: like a persisted store, no shared refs
     set: (k, v) => { o[k] = v; } });
  const sandbox = { Buffer, Date: makeDate(now) };
  const ctx = vm.createContext(sandbox);
  const fnc = vm.runInContext(`(function (msg, node, flow, global, context, env) {\n${n.func}\n})`, ctx, { filename: name });
  const node = { send: (m) => sent.push(m), warn: (m) => warns.push(m), error: (m) => errors.push(m), log() {}, status() {} };
  const result = fnc(msg, node, store(flowCtx), store(globalCtx), store({}), { get: () => undefined });
  return { result, sent, warns, errors };
}

module.exports = { loadFlows, findNode, runFunction };
