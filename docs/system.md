# Heating system

The physical installation behind the data: equipment, hydraulics, and a log of setting changes. Terms follow `GLOSSARY.md`. Use the change log to compare data before and after a change.

## Equipment

- **Heat Pump:** LG Therma V monoblock HM091MR.U44 (9 kW, R32, series 4), with its own internal pump on the primary circuit. Controller (RMC) firmware 3.06.6. The fixed-flow setting cannot go below ~19.5 L/min on this firmware (other 9 kW U44 owners on 3.07.2a can set 8 L/min; see `docs/research/tweakers-lg-therma-thread.md`). Pump capacity % is a hard cap in every pump mode (9 kW U44 forum curve: 40% ≈ 10, 60% ≈ 13.7, 80% ≈ 17.5, 100% ≈ 20 L/min).
- **Buffer:** 80 L vessel, in series in the return (see Hydraulics).
- **Secondary pump:** Grundfos ALPHA1 L 25-40 180. Max head ~4 m, nominal point 1.65 m³/h @ 1.89 m. Modes: proportional pressure ("radiator"), constant pressure ("underfloor"), constant curve I/II/III.
- **Emitters:** convectors (two with a Heat Booster: `heat_booster_links`, `heat_booster_rechts`) + floor heating.
- **Zone control:** Honeywell evohome.

## Hydraulics

```
                 ┌──► ALPHA1 (secondary) ──► convectors + floor heating ──┐
Heat Pump flow ──┤ tee                                                      │
                 └──────────────────► Buffer 80 L ◄─────────────────────────┘ return
                                          │
Heat Pump return ◄────────────────────────┘
```

- The tee is the hydraulic separator between primary and secondary. The buffer sits in series in the return.
- **Primary flow > secondary flow:** the excess hot flow goes via the tee into the buffer and back to the Heat Pump. Effects: Return Temperature rises, Delta T drops, Flow Temperature overshoots its target, and the compressor slows down and stops.
- **Secondary flow > primary flow:** buffer water flows backwards through the tee–buffer branch and dilutes the flow to the emitters.
- **Heat Pump pump off, secondary pump running:** the ALPHA1 pulls ~1000 L/h through the Heat Pump, so `flow_rate` is not zero while idle. Part of the secondary flow may bypass via the tee–buffer branch.
- Keep the tee (or another bypass): it gives the Heat Pump a flow path when the secondary side closes.

## What the monitoring sees

- **Primary side:** Heat Pump Flow Temperature, Return Temperature and Flow Rate, plus the Heat Meter.
- **Secondary side:** only the Heat Booster flow/return pipe temperatures (`booster_1m`). These read lower than the Heat Pump flow when the secondary pump is off, because the pipe cools down. That gap is not mixing in the buffer.
- **Not logged:** secondary pump state, secondary Flow Rate, evohome demand, buffer temperatures.

## Change log

| When (Europe/Amsterdam) | Change |
| --- | --- |
| before 2026-11-06 19:55 | Baseline. evohome switches the ALPHA1 on/off (6 cycles/h). LG pump at its default flow. |
| 2026-11-06 ~19:55 | ALPHA1 runs continuously on constant curve III (fixed max speed). LG pump unchanged. |
| 2026-11-07 (time not recorded) | LG pump mode fixed Delta T 5 K, max pump capacity 45% (lowest). Fixed flow could not be set below ~19.5 L/min. |
| 2026-11-07 ~10:55 | ALPHA1 to constant curve I. LG: pump mode fixed flow at lowest setting (19.5 L/min) with max pump capacity 55–60% as the effective cap, control on outlet, water hysteresis −2/+4, heating range widened (~target ± 8 K), idle pump interval 2 min on / 20 min off. Silent mode unchanged (still forced on). |
| 2026-11-07 ~11:02 | Silent mode off: Node-RED inject now writes coil 2 = false every 1 s (was true every 1 s since before monitoring; silent was permanently on). Reason: silent below ~3–4 °C gives more defrosts, and the 9 kW U44 ices fastest. |

### Baseline (2026-11-06 17:40–19:45, outdoor 6–8 °C, ~2 h of data)

- **Space Heating:** Flow Rate 1,200–1,380 L/h. Delta T 3–3.8 K (target 5).
- **Return Temperature:** Heat Pump return 40–45 °C vs Heat Booster return ~33 °C. The excess primary flow is short-circuiting through the buffer.
- **Flow Temperature:** overshoots the target (43 → 48 °C). The compressor slows from 40 to 23 Hz and stops.
- **10-minute pattern:** Return Temperature drops 3–4 K every 10 min as the secondary pump starts (evohome).
- **Compressor Cycles:** 29, 12 (DHW) and 41 min. COP Space Heating 3.09 (provisional, 69 min).
- **Idle with the pump running:** Delta T reads −0.5 to −1 K, a possible sensor offset.

### Next steps

1. **Settle period:** 2026-11-07 ~11:00 onwards. Settings changed together (pump/hysteresis ~10:55, silent off ~11:02), so compare as one package.
2. **Compare 2–3 days after against the before windows** at similar outdoor temperatures:
   - Before: baseline 11-06 17:40–19:45, and curve III only 11-06 19:55 → 11-07 10:55.
   - Metrics:
     - Heat Pump Flow Rate when running and when idle. Idle flow shows what the ALPHA1 on curve I drives through the Heat Pump; it was ~1000 L/h on curve III.
     - Delta T. Return Temperature against Heat Booster return.
     - Flow Temperature overshoot against target.
     - Compressor Cycles: count, length, and stops/h.
     - Compressor Hz at start (start peak).
     - Defrosts/h at outdoor temperatures below ~5 °C.
     - COP Space Heating per day.
3. **Watch for side effects of silent off:**
   - Hourly stops after ~1 h at low Hz (oil-return spike). Fix: target +1–2 K.
   - Higher start peaks.
   - CH14 errors (`error_code`) from the lower pump capacity.
4. **If Delta T is still well under 5 K,** lower the ALPHA1 further (PP1/curve I is the minimum) or the LG capacity % (not below ~45%, CH14 risk).
5. **After that,** lower the Heating Curve by about 1 K/day while rooms stay warm.
6. **Firmware:** ask the installer or LG for RMC 3.07.x plus an outdoor-unit software check, so fixed flow can go below 19.5 L/min.
7. **Silent mode in spring:** re-enable it above ~5–6 °C (outdoor-temperature rule in Node-RED).
8. **Buffer piping** (4-port buffer or low-loss header): only if the steps above don't help enough.

Measured 2026-11-06/07 (before the 11-07 changes): with the LG pump off, the ALPHA1 (curve III) alone drives ~1000 L/h through the Heat Pump; with the LG pump on, flow never drops below ~1180 L/h. The ALPHA1 sets the floor of the primary flow, so lower it before expecting a lower LG flow.

Open: does the floor heating have its own mixing valve or pump? Is a newer RMC firmware (3.06.9b / 3.07.x) available via the installer or LG?
