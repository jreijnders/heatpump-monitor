# LG Therma V Modbus register map

Slave 1, 9600 8N1. Addresses are 0-based protocol addresses (manual numbers 3xxxx/4xxxx are 1-based: address = manual - 30001 etc.).

## Sources and trust

- No LG primary PDF could be fetched. Maps below are community transcriptions of the LG manual. LG's own Modbus register numbering is 1-based.
- [S1] basti242 HA wiki "LG Register documentation" (transcribes LG manual; has known typos): https://github.com/basti242/homeassistant_lg_therma_v_modbus/wiki/LG-Register-documentation
- [S2] basti242 `modbus_lg_heatpump.yaml` (values confirmed on a device; wins over wiki on conflict): https://github.com/basti242/homeassistant_lg_therma_v_modbus
- [S3] Older Node-RED gist (regs 0-12 only, no pressure): https://gist.github.com/moench13/c7e8239a7f21b7926e2d3b140bbe8e4d
- [S4] HA forum: https://community.home-assistant.io/t/lg-therma-v-heat-pump-modbus-control/496233 ; PyPI `lg-heatpump-modbus` (not readable here)
- Generation caveat: TapHome says 2021+ units via PMBUSB00A gateway use different addressing. Energy regs 36-45 and holding 23 only on units built from ~Dec 2023.

## Discrete inputs (FC02), 0-based

| Addr | Name | Notes | Src |
|---|---|---|---|
| 0 | Water flow status | 0 = OK, 1 = flow too low | S1,S2 |
| 1 | Water pump status | | S1,S2 |
| 2 | External water pump status | | S1,S2 |
| 3 | Compressor status | | S1,S2 |
| 4 | Defrost status | | S1,S2 |
| 5 | DHW heating status | | S1,S2 |
| 6 | DHW tank disinfection status | | S1,S2 |
| 7 | Silent mode status | | S1,S2 |
| 8 | Cooling status | | S1,S2 |
| 9 | Solar pump status | | S1,S2 |
| 10 | Backup heater step 1 | | S1,S2 |
| 11 | Backup heater step 2 | | S1,S2 |
| 12 | DHW boost heater | | S1,S2 |
| 13 | Error flag | | S1,S2 |
| 14 | Emergency operation available (space heating/cooling) | | S1,S2 |
| 15 | Emergency operation available (DHW) | | S1,S2 |
| 16 | Mixing pump status | | S1,S2 |

S3 reads only qty 11 from 0 (0-10).

## Input registers (FC04), 0-based

Temps are signed int16 /10 unless noted (assume signed for outdoor/suction temps; sources do not state signedness explicitly).

| Addr | Name | Unit/scale | Src |
|---|---|---|---|
| 0 | Error code | raw | S1-3 |
| 1 | ODU operation cycle | 0 standby, 1 cooling, 2 heating | S1 |
| 2 | Water inlet temp | /10 degC | all |
| 3 | Water outlet temp | /10 degC | all |
| 4 | Backup heater outlet temp | /10 degC | all |
| 5 | DHW tank temp | /10 degC | all |
| 6 | Solar collector temp | /10 degC | all |
| 7 | Room air temp circuit 1 | /10 degC | all |
| 8 | Current flow rate | /10 **L/min** | S1,S2 (existing flow labels it m3/h: wrong) |
| 9 | Flow temp circuit 2 | /10 degC | all |
| 10 | Room air temp circuit 2 | /10 degC | all |
| 11 | Energy state input | enum | all |
| 12 | Outdoor air temp | /10 degC | all |
| 13 | **Water pressure** | /10 bar | S1,S2 |
| 14, 15 | not documented | | |
| 16 | Liquid-gas (liquid line) temp | /10 degC | S1,S2 |
| 17 | not documented | | |
| 18 | Suction temp | /10 degC | S1,S2 |
| 19 | Hot gas (discharge) temp | /10 degC | S1,S2 |
| 20 | Temp before evaporator | /10 degC | S1,S2 |
| 21 | Temp after evaporator | /10 degC | S1,S2 |
| 22 | High (condenser) pressure | /100 bar | S2 (wiki mislabels) |
| 23 | Low (suction) pressure | /100 bar | S2 |
| 24 | **Compressor speed** | wiki: Hz; YAML: raw x60 = rpm (same thing: Hz x 60) | S1,S2 |
| 36-37 | Instantaneous total energy (hi,lo word) | Wh | S1 (>= Dec 2023) |
| 38-39 | Accumulated total energy | Wh | S1 |
| 40-41 | Instantaneous heating energy | Wh | S1 |
| 42-43 | Accumulated cooling energy | Wh | S1 |
| 44-45 | Accumulated DHW energy | Wh | S1 |
| 9997 | Device group | 0x80/83/88/89 | S1 |
| 9998 | Device info | 0 split, 3 monobloc, 4 high temp, 5 medium temp, 6 system boiler | S1,S2 |

(Wiki lists 39997/39998 in 1-based manual numbering; YAML uses 9998 0-based.)

## Holding registers (FC03/06), 0-based

| Addr | Name | Unit/scale | Src |
|---|---|---|---|
| 0 | Operation mode | 0 cool, 3 auto, 4 heat | S1 |
| 1 | Control method | 0 outlet water temp, 1 inlet water temp, 2 room air | S1 |
| 2 | Target temp circuit 1 | /10 degC | S1,S2 |
| 3 | Room air setpoint circuit 1 | /10 degC | S1,S2 |
| 4 | Shift value auto mode circuit 1 | K, int16 (-5..+5); negatives as 65536+n | S1,S2,S4 |
| 5 | Target temp circuit 2 | /10 degC | S1,S2 |
| 6 | Room air setpoint circuit 2 | /10 degC | S2 (wiki omits row) |
| 7 | Shift value auto mode circuit 2 | K int16 | S1,S2 |
| 8 | DHW target temp | /10 degC | S1,S2 |
| 9 | Energy state input | 0 unused,1 forced off,2 normal,3 on-recommend,4 on-command,5 on-command step2,6 on-recommend step1,7 energy saving,8 super energy saving | S1 |
| 23 | Compressor modulation / power limit | 0 unused, 40-100 | S1 (wiki, 1-based 40024 => addr 23) |
| 24 | hp_power_limit (kW) | YAML puts it at 24 | S2 |
| 9998 | Product info | | S1,S2 |

## Coils (FC01/05), 0-based

0 enable heating/cooling, 1 enable DHW, 2 silent mode set, 3 trigger disinfection, 4 emergency stop, 5 trigger emergency operation. (S1; S3/S2 differ on 4/5.)

## Conflicts / uncertain

- Holding 2 "target temp circuit 1": no source says whether it is the fixed setpoint or the curve-computed value in auto mode. In auto mode (op mode 3) it is most likely the user-set/offset base; the curve result is NOT exposed. Verify empirically: log holding 2 vs input 3 (outlet) across outdoor temps.
- Compressor reg 24: wiki "Hz" vs YAML "x60 U/min". Existing flow labelled it "inverter"; fine. Verify magnitude (expected ~20-100 Hz raw if Hz; if raw is already Hz, x60 gives rpm).
- Existing Node-RED labels vs map: [16] "pipe in" = liquid-gas temp; [17] undocumented (flow's /1000 suspect); [18] suction ok; [19] flow calls it fan speed but map says hot gas (discharge) temp /10; [20] "hx temp" = temp before evaporator; [21] after evaporator (unread); [22]/[23] pressures /100 bar: matches S2; [24] compressor Hz. Reg 19 "fan speed" is likely WRONG: check values (~50-100 degC => discharge temp).
- Flow rate unit: L/min /10 (S1,S2); flow's m3/h label likely wrong. Sanity check: COP flow ~ 15-25 L/min for 9 kW.
- No documented ODU fan speed, calculated target flow temp, or instantaneous electrical power (except energy regs 36-45 on >=Dec 2023 units). Ext. meter in flow (hp_electric) already covers power.
- Signedness of temps not stated; treat as int16.
- Wiki typos: pressure labelled degC, 30023/30024 duplicate name, missing 30015/16/18 (addr 14,15,17), missing 40007.

## Recommended additions for COP/cycling analysis

1. Input 13 water pressure (/10 bar): low-pressure faults, flow plausibility.
2. Input 24 compressor frequency: cycling, modulation depth, load; discrete 3 for on/off edges.
3. Discrete 4 defrost, 5 DHW heating, 8 cooling, 10/11/12 backup + boost heaters: exclude from space-heating COP; detect heater contamination.
4. Input 1 ODU operation cycle: mode segmentation.
5. Input 19 discharge temp, 16 liquid temp, 22/23 pressures: refrigerant health (already read).
6. Holding 2/4 (target, shift) and 0/1 (mode, control method): heating-curve tuning; log, but curve-calculated target flow temp is unavailable.
7. Input 36-45 energy counters if firmware supports (probe; cross-check vs external meter).
8. Discrete 0 water flow alarm; 13 error + input 0 error code.
