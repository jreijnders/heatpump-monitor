# Tweakers "LG Therma (Monoblock) warmtepompen topic – Deel 1": findings

Source: https://gathering.tweakers.net/forum/list_messages/2017448 (212 pages, Sep 2020 – 2026, Dutch). Mostly 5/7/9 kW U43/U44 monoblock owners (R32), later also R290 and 12–16 kW. These are forum claims, not verified on our unit. Settings and limits differ by series (U43 vs U44, R32 vs R290), size and firmware. `pNNN` = forum page (±1). Names are forum handles.

Coverage: all 212 pages, read 2026-11-07 (41 via archive.org, the rest live). Most of pages 3–131 were keyword-filtered rather than read word for word.

## Pump, flow, Delta T

### Pump settings (installer menu)

- **Modes:** optimal flow, fixed pump capacity %, fixed flow, fixed Delta T (Set/Release: no difference, p72). Older U43: only capacity % or Delta T.
- **Capacity % is a hard cap in every mode** (jeroenst p205; Jojan265/paQ p48), and it is the speed used at start-up and post-run. The sequence (p134/p135, p183) is:
  1. Defrost: 100%.
  2. Start-up and post-defrost, ~10–15 min: capacity %.
  3. Then fixed/optimal flow or Delta T.
- **Set capacity % ≈ the fixed flow** (PeteSplit p185/p205: kW × 3 L/min, 5 kW ≈ 15 L/min ≈ 45–55%). butterkeks p129/p134: 17 L/min + 50% stopped overshoot and Hz swings; 45% = 13.6, 55% = 17 L/min. Opposite school (paQ p101/p126, tebogo p96): fixed flow + cap 90–100% to get through the peak.
- **Pump power is small:** 5 kW 14 W at 15 L/min, 28 W max; 12 kW R290 145 W at 100% vs ~50 W at 11–12 L/min. At 10 Hz it matters: pump 90% (60 W) COP 4.2 vs 50% (10 W) COP 4.8 (ingmar89 p155).
- **Idle pump interval:** "pump setting in heating" on/off, default 2/1, 3/3, 2/28 or 3/30 depending on version. Applies only after thermo-off. The compressor only starts during the on-time. LG guideline ≤3 starts/h, so use ~20–28 min off (p143, p204). ≥2–3 min on is needed for correct temperature sampling.

### Flow limits per size

- **5/7/9 kW U44:** one pump, ~8–26 L/min. Low-flow switch (reed, separate from the tacho flow value) trips below ~7.5–8 L/min → CH14 (p48, p78, p200). Fixed-flow minimum is firmware-dependent: RMC 3.07.2a: 8; after an ODU update lower (p200/p202).
- **12–16 kW:** practical minimum ~15–17.5 L/min (BernieW p129, tinusit p91, Cees-JanH p200: optimal bottoms at 16.9; cap 40% → CH14).
- **Unit flow test at start:** the firmware assumes a minimum by unit type (~15 L/min) even where the unit would cope with less (detviaklm p161). A 5 kW must reach ~18 L/min (p122).
- **Measured pump curves:**
  - 9 kW U44, 25 mm pipe: 40% = 10, 60% = 13.7, 80% = 17.5, 100% = 20 L/min (p167).
  - 5 kW: 50% = 8, 65% = 14, 100% = 25.
  - 12 kW: 100% ≈ 25–34. 16 kW: 80% = 31, 100% = 40.
  - LG internal pressure drop is much higher than spec (KMarco p29).
- **Other CH14 causes:** closed zones/TRVs, clogged filter/strainer, pressure <1.8 bar, air, failed reed switch, too-cold return after a defrost (p77, p94, p150, p162). It usually clears within 1 min, and a 1-min poll misses it.

### Optimal flow vs fixed flow vs fixed Delta T

- **Optimal flow** targets Delta T 5 (5–10 K per the service manual); LG training recommends it. Problems:
  - After start it drops fast (22 → 8 L/min) → overshoot.
  - At 10 Hz + ~8 L/min the hourly oil-return spike can't be shed → stop (JansenSE p157).
  - CH14 after defrosts or on zone closure.
  - With a buffer or second pump: back-mixing, buffer flow reversal (rooiekool p100, PeteSplit p161, p211). "Optimal flow is hopeless with a second pump."
- **Fixed Delta T:** CH14 after defrost and trips on the oil-return peak (Jojan p72); cycling (tebogo p151). The LG can't hold Delta T 5 at low power, because the pump doesn't go low enough and ends near 3.5 K (walter_321 p205). One user fixed an optimal-flow stall below −2 °C with fixed Delta T 5 (p195).
- **Fixed flow:** most stable for the majority. Examples:

  | Flow (L/min) | Unit / result | Source |
  | --- | --- | --- |
  | 12.3–12.5 | cap 90%, COP 5.7–8 | paQ |
  | 15 | 7 kW (jschel ~15) | Jojan265 |
  | 16.5 | — | tebogo |
  | 17.5 | 12 kW | Casper |
  | ~20 | 9 kW | jschel |

  Below ~12 L/min the hourly oil-return peaks appear when not in silent mode (paQ p100).
- **Low Delta T vs COP:** disputed.
  - walter_321: COP 3.8 at Delta T 2.5 K / 11 L/min vs 4.6 at 4 K / 14 L/min (p193).
  - Others say compressor Hz dominates (p192/p197).
  - Same Ta → same kWh at any flow (PeteSplit p162).
  - At 13 L/min / Delta T 7 / cap 40%, COP was unchanged and pump power halved (p165).
- **Primary vs secondary with a buffer or second pump (consensus):** LG primary slightly ≥ secondary. PeteSplit primary 18–20, secondary 13–15. BernieW ideally net-zero buffer flow. Parallel buffer: external pump ≥ LG pump (p50, p161). A parallel buffer still mixes even with lower secondary flow (BjornHero p155).

## Start-up peak, overshoot, thermo-off

- **Consensus:** the start is hard-coded firmware behaviour, and LG says it is "normal" and won't fix it.
  - Starts at ~30 Hz, then ramps to (near) max within ~1 min; pump at capacity % for ~10–13 min.
  - Peak ~15 min (range 10–40). 5 kW gives >9 kWth for 15 min and overshoots ~10 K (M_AW p29).
  - Hz is chosen from outdoor temperature, setpoint and Ta at start. Changing the target mid-start does nothing, except silent mode (Devil/paQ p119).
  - The peak scales with the target (p107). After a defrost, 40 → 10 Hz takes 16–20 min.
- **Mechanism (R32 U44):** rising Hz pushes hot gas above ~75 °C → flash-gas/refrigerant injection → the compressor races ~15 min (butterkeks p161/p200). A condensing temperature ≥12 K above Ta for 10 min opens EEV1 → Hz surge up to 130 Hz (Jojan265 p135).
- **Thermo-off rules:**
  - The compressor stops only when water exceeds the upper hysteresis for >3 min (LG training, Jojan265 p49).
  - During the start peak, Ta may exceed target by +4 to +5.5 K regardless of the set hysteresis; not after ~10 min (p95, p127, p185).
  - LG only throttles back once the error is >2 K (teunos93 p204).
- **Hysteresis (water):**
  - Lower value = turn-on offset, upper = thermo-off.
  - R32 range −4..+4 in 0.5 steps, minimum total gap 2 K (p188). Older firmware has types 0–3 (type 2 = ±4, type 3 = ±1).
  - Working sets: −2/+4 (most common), −1.5/+3.5 (Jojan p29: 0/+4 trips, −1.5/+3 works 50% of the time), −4/+4 (Barbrady, tebogo, kazemo), −4.5/+3.5 (teunos93).
  - Lower too tight → still warm at restart → overshoot trip (p42). Don't go below ±3 (p204).
  - After a recent software update, 0/+2 removed the full-power start (walter_321 p202/p203).
- **Heating temperature range** (e.g. 25/40) is a hard limit including hysteresis. Set it ≈ target ± 2× hysteresis, e.g. 15/40 (PeteSplit p153).
- **Fixes in use:**
  - **Target ramp via Modbus holding 2 / AI shift:**
    - paQ ThermaV_overshoot_logic (github paqpaqpaq, fork fonske): new target = Tr + 5·(Tr−17)/(curve−25), min Tr+4. ESPHome version: −4 for 3 min, then −3/−2/−1/0 at 4 min each → peak 36–38 Hz instead of >52.
    - Devil script (p50): +1 K up to 3× when hysteresis is about to be exceeded, predicted from the last 20/5 min; +2 K for 1 h after DHW.
    - Pixal Node-RED (p92): every 20 s for 30 min, flow−target >4 → +1, <3 → −1.
    - teunos93 soft-start (p184).
    - esp-lg-control (github joep2000): target 0.5 K below return while heating.
    - Mixed results on large water volumes. butterkeks says lowering the target disturbs LG's integral controller, so lower pump % instead.
  - **Energy State 7** for the first 45 min: −2 K water target, peak 2300 → 1900 W (water mode only, JosHen p96).
  - **Silent mode for the first minutes:** 2 min → 46 Hz peaks, 4 min → 36 Hz. Switching silent→normal at 15 min caused overshoot and 1.5 h off (p50).
  - **Inlet control for the first ~3–10 min, then outlet** (butterkeks p197/p200, tebogo p198).
  - **Open all zones/TRVs during start or defrost** (paQ p37/p47, sparkly p52/p118). Keep one radiator always open (rykers p9, BernieW p91).
  - **Heat Booster/fan emitters that boost on the start-up water rise:** fan firmware v2.7.0 runs fans at 100%, which stopped cycling at a 27 °C setpoint (Bazz0847 p163). Speedcomfort switch threshold ~35 °C (older ~27 °C), lowered by swapping the KSD301 to 25 °C or a 45–50 kΩ parallel resistor (p102). **Relevant to our Heat Boosters.**
- **Stuck-low failure:** after the peak the unit holds ~1500 W, 34–35 °C against target 38, until the next defrost. Fixes: RMC off/on, target +2, hysteresis 0/+3 (p132–p134).

## Oil return cycle (ORC, "smeerpuls")

- **Trigger:** a fixed ~1 h timer that runs while the compressor stays below a threshold (~20 Hz on 5 kW R32, ~25 Hz on 12 kW R290, or <1000 W). It resets when the compressor goes above (teunos93/PeteSplit p182–p184).
- **Effect:**
  - ~30–60 s at ~40–45 Hz, ~1500 W on 5 kW. It forces 24–33 L/min.
  - Flow Temperature +~5 K. If this exceeds the upper hysteresis for >3 min, or a setpoint was just raised → stop. At −2 hysteresis the restart can take >2 h.
- **No ORC in silent mode** (most reports), but R290 silent can block it (DIP "limited" fix).
- **Fixes:**
  - Target +1–3 K so the compressor stays >20 Hz.
  - Every 50 min raise the AI shift until above the threshold, then lower.
  - Flow high enough that the ORC stays within +4. JansenSE minimum flow for a 9 kW: COP 6 → 130/ΔTmax, COP 4.6 → 100/ΔTmax, COP 3 → 65/ΔTmax (ΔTmax 4 → 32/25/16 L/min).
  - Wider upper hysteresis.

## Control point, curve, AI

- **Outlet vs inlet: no consensus.**
  - Inlet control: runs 40–45 min instead of 8 (Casper p21), but a higher and longer start peak (~200 Wh more, PeteSplit p185) and drift into the upper hysteresis (p64).
  - After a defrost, inlet control gives a bad peak (cold inlet), so use outlet in defrost season (kuv p112).
  - PeteSplit 5 kW: outlet COP 4.6 / 6.02 kWh vs inlet 4.4 / 6.63 kWh (p143).
- **Water temperature setting "Manual"** ignores the curve: 750–1000 W vs 350–400 W on Auto (p127).
- **Curve:**
  - 2 points, followed in AI or water-auto. Targets are whole °C (written 28.9 reads back 28). The AI target is truncated (31.9 → 31), giving up to 1.4 K error; the RMC has no integral action (p191).
  - Extend both ends (top ~18 °C, bottom ~−20 °C). Behaviour outside the range is disputed: setpoint stays flat (paQ p72) vs no heating above the top point (SalexSun p71, p121, SCG p142).
  - Typical floor/mixed curves: −10/30–36 → 15–18/22–28. Our 43 °C at 6–8 °C is high by these standards.
  - ~+1 K lift ≈ −2.5% COP (Weishaupt); ~0.1 COP per K Ta (Jojan p70). Setpoint −1 K: COP 4.4 → 5.3 (paQ p122). Lowering the flow target 1–2 K helps start-up COP more than hysteresis does (jschel p166).
  - **Method:**
    - Water-only, fixed water temperature for 1.5–2 days at ≥2 outdoor temperatures, change ≤1 K/day.
    - Shortcut: on a mild day take the holding temperature, add 10 K, and set that at −10 °C (paQ p102).
    - Outdoor input smoothing: 30% now + 70% 8 h mean + wind chill (butterkeks p139).
  - Defrost raises measured outdoor +3 K → lower AI target → overshoot (p53). The outdoor sensor also drops 2–4 K at compressor start (p103).
- **Modbus control:**
  - Holding 2 target only works in heat ("Warmte") mode and is ignored in AI. The AI shift (holding 4) is only writable in AI mode. Negative shift values are written as 65536 + n.
  - Holding 40004 room air temperature is writable in air mode, i.e. a virtual thermostat (butterkeks p121).

## Thermostat / evohome

- **evohome TPI via BDR91** → ~6 starts/h, a known cycling cause. BernieW: evohome is the "worst combination" with a heat pump.
- **BDR91T fix:**
  - Needs evohome fw 02.00.19.33, DIP 8 on, thermostat type "heat and cool", and a full reset (link, heat-pump module, zone) before the 1/h, 30-min rest setting takes effect (p139–p146, Barbrady p191).
  - The RMC setpoint still gates heating.
  - Terminals: 24 heat, 25 DHW (230 V, never both); 21 common, 23 cool on some versions.
- **Barbrady (p191/p193):** evohome removed from the Heat Pump, heat mode 27 °C −4/+4, external pump on dry contact A2 → 6 stops/h became 0, runs 24/7 at 400–500 W.
- **Time relay** holding demand 20 min after the thermostat stops: long runs, no icing (1wouterb p28, Joris407 p75).
- **External thermostat (DIP 8):**
  - The Heat Pump modulates on water only; air becomes on/off. The RMC is greyed out, so the pump interval is lost.
  - The input is high-impedance and may need a ~100 kΩ pull-down. An SSR can leak and create phantom demand.
  - JanAllElectric: a 0.2 K band thermostat lifted SCOP 3.2 → 4.3.
- **RMC air hysteresis** has a 2 K minimum window. The RMC room sensor reads 1–1.5 K low.

## Buffer / hydraulics

- **Series buffer in the return + bypass (recent consensus):** "80 L more than enough", doubles as defrost volume. 40–50 L in series works for several.
  - LG manual minimum system volume (R32): 20 L with backup heater, 80 L without. Series ≥50 L for 12–16 kW.
  - LG manual p40 shows only 4-port parallel buffers; some dealers demand that for warranty (p191). LG Korea: an always-open floor needs no buffer.
- **Parallel buffer:**
  - Needs secondary ≥ primary, or it fails (p63). Optimal flow + parallel buffer → reversal or mixing.
  - Mixed results: 160 L removed start problems (Casper p79), 150 L did not help (Devil p31).
- **Start-stop loss study:** 12.5% → 0.8% at 0.5 m³ (p81). A 50 L buffer buys only ~2.5 min at 1 L/s and 2 K.
- **Defrost:** during defrost ~40% via the house, 60% via the buffer (markklok p109). Defrost stops when water reaches ~14 °C. A monoblock needs return >15–16 °C within 15 min.
- **Pumps:** some stop the secondary pump at defrost start (disputed). A Δp bypass needs ≥0.45 bar or CH14 (tebogo p140). A mixer valve disturbs flow; fix its position (p100).

## Silent mode / defrost

- **Silent caps:** about 36 Hz (7 kW), 40 (9 kW), 45 (5 kW), 52 (12 kW); ~40–60% of power (3000 → 1800 W). Silent output 7 kW: 10 °C 6.0 kWth, 0 °C 4.8, −2 °C 4.2 (p140).
  - RMC power-off clears the silent flag; a Modbus off keeps it.
  - Silent at a low-Hz moment can keep the Hz cap for hours; Node-RED setting silent whenever <15 Hz caught defrosts (butterkeks p111). **Check our coil-2 write.**
- **Silent rules:** off below ~2–3 °C or RH >96%, on above ~5–7 °C. Better: switch on suction/dew-point margin.
  - tebogo: off if suction <0 °C for 5 min, on >1.5 °C.
  - teunos93: off if margin <−0.3 and suction <0.
  - Silent after a defrost: 45 → 90–120 min intervals (Jojan265), contested by others (paQ, gerlag).
- **Defrost trigger:** suction ≥~10 K below outdoor and <0 °C (Jojan265 p114/p164). Predictor: outdoor − suction >10 K and >45 min since the last.
- **Defrost behaviour:**
  - 2–7 min, pump 100%, water −4–5 K. Costs 0.33–0.5 kWh thermal (7–9 kW) or ~6% of output.
  - Blocked below 4 °C leaving water; the compressor won't start below 15 °C water.
  - COP between defrosts falls 3.5 → 1.2–2.2 as ice builds. The 9 kW U44 shares the small 5/7 kW evaporator and ices fastest (p194).
  - LG firmware drops the target ~2 K after a defrost (p169).
- **Tricks:**
  - Curve −2 K below 4 °C (SalexSun).
  - +1 K if a defrost recurs within 50 min (BernieW).
  - +3 K if <15 Hz within 80 min after a defrost, to trigger the next defrost sooner (SalexSun p195).
  - Avoid night setback (more icing).
- **Hot-gas defrost:** DIP / "Rapid Defrost Mode"; the manual advises disabling hot gas with an external pump (Kip p202).

## Minimum modulation / COP

- **~30–40 W/Hz:** 10 Hz 270–360 W (5/7/9 kW), 15 Hz 560; 12 kW minimum 480–650 W.
  - Minimum power rises with outlet temperature: 24 °C 320 W, 28 °C 360, 32 °C 460.
  - The compressor only drops below ~30 Hz once high pressure (bar) < flow (L/min) (mdvmine p210).
- **COP reports:**
  - 300–330 W at COP 5.5–6.5 when mild.
  - SCOP 4.4–4.95 (Jojan265, KarsS, teunos93).
  - Cold, humid ~0 °C: 2.5–3.
- **Metering:**
  - Daily COP = Σheat/Σelec, including negative defrost heat.
  - LG electrical readout overstates by 7% to 2× (lookup table, ~300 W shown when off).
  - LG temperature sensors are 0.5–2 K off (resolution reported as 0.3 or 0.8 K) and unreliable at Delta T <2 K. In paste + insulation they agree within 0.1 K.
  - The flow register reads ~5 L/min (or a constant) with the pump off: filter with the pump flag.

## Backup heater / DHW

- **Backup heater:**
  - Fires ~2.5–3 kW for ~5 min at defrosts on units with a kit; disable with DIP 6+7 off. Adds only ~1.5 K, so it barely helps a defrost.
  - Threshold default −5 °C (users: 0 or −15).
- **DHW:**
  - "Min" is a differential below the comp limit (55–58 − 15 = start at 43 °C). DHW priority is needed, otherwise only the element heats.
  - After a DHW run the unit can idle in hysteresis for 2–4 h; workaround: shift +5 until the compressor starts.
  - R32 DHW cap ~58 °C. A run stops heating for 1.5–2 h.

## Modbus practice

- **Polling:**
  - ≥15 s is safe. teunos93 reads 3 s/10 s/60 s with no CH03; others got CH03 at 5 s.
  - CH03 (no comms for 3 min) needs a power cycle. Losing Modbus does not stop the Heat Pump. One master only.
  - ESPHome `reboot_timeout: 0s`, otherwise a Wi-Fi loss can power the unit off.
- **Wiring:** isolated RS485 (cheap USB → CH03), 120 Ω at the last node, shield grounded at one end.
- **Registers beyond `lg-therma-v-modbus.md`:**
  - Input 18 suction, 19 discharge (not fan), 20 condenser/HEX/inverter temperature (labels disputed).
  - Input 47 pump power (W), 49 thermal power (R290), 37 electrical power (inaccurate).
  - Holding 23 compressor limit % (R290, newer firmware); coils 6/7 and holding 24/25 power limit (mixed results).
  - Input 30012–30015 may error. Manual register numbers are 1-based.
- **Firmware:**
  - Installer PIN = RMC firmware digits (3055, 3076). RMC 3.0.6.9b: "unit setting" menu, config export to SD. Firmware only via LG (SD/USB).
  - Settings occasionally reset spontaneously, and air+water can fall back to water-only.

## 9 kW U44 (our model, HM091MR.U44)

- **Pump curve, 25 mm pipe** (Antonl91 p167): 40% ≈ 10, 60% ≈ 13.7, 80% ≈ 17.5, 100% ≈ 20 L/min. Others: range 7.8–22 L/min (gerlag p70); 100% about 22–26. CH14 below ~7.5–8 L/min.
- **Typical settings:**
  - Fixed flow ~15 L/min for 7 kW, ~20 for 9 kW (jschel p203).
  - tebogo 9 kW (COP ~5): optimal flow, cap 50%, hysteresis −2/+4 (p138/p139).
  - mjvdkolk 9 kW: fixed 15 L/min gave Delta T ~3 vs 5 at 8 L/min (p126).
- **Fixed-flow minimum depends on firmware:**
  - RMC 3.07.2a: minimum 8 L/min (mdvmine p200; he guesses it may also depend on which of the two pump brands is fitted).
  - Minimum 11 L/min on a 7 kW R290 (walter_321 p187).
  - Our RMC 3.06.6: ~19.5 L/min.
  - A wrongly reported unit size (7 kW shown as 16 kW) was fixed by an LG outdoor-unit software update (walter_321 p193/p202).
- **Evaporator:** shares the small 5/7 kW frame, so it ices fastest (teunos93/jschel p194). 10 Hz ≈ 292–360 W. Silent cap ~40 Hz. Defrost costs ~0.5 kWh thermal.
- **Backup heater:** with a 6 kW kit it starts at every defrost (p210). Ours has no kit (check).

## Firmware updates (RMC)

- **Official route:** installer or LG service via LG Nederland (p178, p190: a technician updated 3064 → 3069 with a stick in the back of the RMC). LG refuses consumers. Note: this is RMC firmware only; the outdoor/IDU board firmware is separate and LG-only. The IDU board version is not readable.
- **Self-install (it happens):**
  - butterkeks got 3069b "from a German forum" and flashed it via micro-SD (p191/p199). Settings kept, consumption history wiped.
  - kazemo had 3069b on his laptop but hadn't flashed yet (p200).
  - An English-speaking user notes others got files from LG and asks why they aren't shared (p178).
- **What 3.06.9 adds:**
  - A "unit setting" menu (PIN 3069).
  - Config export to SD as JSON (max pump rpm and Delta T are not editable through it).
  - A lower-limit option for heating temperature.
  - Thermal-power display.
  - No new Modbus registers.
- **External reports:**
  - HaustechnikDialog: a user got 3.06.9 from LG support. Install: FAT32 4 GB micro-SD, insert, press the software reset button, remove the card when the power LED lights. https://www.haustechnikdialog.de/Forum/t/266636/LG-Therma-V-Firmware-update
  - Photovoltaikforum: an upgrade wiped all settings, so write them down first. https://www.photovoltaikforum.com/thread/241473-bugs-und-tweaks-f%C3%BCr-die-lg-therma-v-r32/?pageNo=19
- **Risk:** the update overwrites the RMC's NAND filesystem (U-Boot + uEnv.txt from SD, p191). The PREMTB100 GPL source is on LG's site but contains nothing about the control logic (p10).

## Our installation (see `docs/system.md`)

**Measured 2026-11-06/07 (LG flow sensor):**

| State | Flow |
| --- | --- |
| LG pump off, compressor off | 1000–1010 L/h (≈16.7 L/min) through the Heat Pump |
| LG pump on, compressor on, minimum | ~1180–1190 L/h (≈19.7 L/min) |
| LG pump on, 100% (start/defrost) | ~1720 L/h |

- The ALPHA1 on constant curve III is pulling water through the Heat Pump via the series return buffer. This, not an LG limit, is why the LG fixed flow can't go below ~19.5 L/min.
- Delta T while running: 1.4–2.7 K.

**Actions:**

1. Lower the ALPHA1 (curve I or low proportional pressure) first, then the LG flow: fixed ~12–15 L/min, capacity % ≈ the same flow. Keep primary ≥ secondary. Avoid optimal flow and fixed Delta T with a second pump.
2. Water hysteresis about −2..−4 / +3.5..+4. Heating range ≈ target ± 2× hysteresis.
3. Check what our Node-RED writes to coil 2 (silent) every 1 s, and how it interacts with defrost and start-up.
4. Lower the curve (43 °C at 6–8 °C is high) and extend the ends to ~18 / −20 °C.
5. Check whether evohome switches the Heat Pump itself. If so, use a BDR91T at 1/h, or take evohome off the Heat Pump.
6. Heat Booster threshold/firmware: boosting on the start-up water rise helps absorb the peak.
7. Watch for the hourly ORC once running at minimum Hz: target +1–2 K or enough flow.
