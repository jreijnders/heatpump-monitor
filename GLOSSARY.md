# Heatpump Monitor

Collecting and analysing measurements from a home heating installation built around an LG Therma V monoblock heat pump, to assess and improve its performance.

## Language

### Equipment

**Heat Pump**:
The LG Therma V monoblock unit that produces heat for space heating and domestic hot water.
_Avoid_: HP, unit, LG

**Electricity Meter**:
The dedicated kWh meter measuring electrical energy consumed by the Heat Pump only.
_Avoid_: power meter, kWh meter, P1

**Heat Meter**:
The Kamstrup Multical measuring thermal energy delivered by the Heat Pump to the heating circuit.
_Avoid_: Multical, energy meter, warmtemeter

**Gateway**:
The Raspberry Pi running Node-RED that reads the Heat Pump, Electricity Meter and Heat Meter and publishes their readings to the Broker.
_Avoid_: Pi, Node-RED box

**DHW Heater**:
The electric resistive heater in the DHW tank that boosts DHW above what the compressor reaches or runs the disinfection cycle. Part of the Heat Pump (its electricity is on the Electricity Meter), not a Heat Booster.
_Avoid_: booster heater, immersion heater, backup heater (that is the space-heating one)

**Backup Heater**:
The Heat Pump's electric resistive heater for the heating circuit (stages 1 and 2), used when the compressor alone cannot meet demand.
_Avoid_: bijverwarming, auxiliary heater

**Heat Booster**:
A fan unit mounted on a radiator that increases its heat output at lower water temperatures. There are two, identified by location (rechts, links); they publish to the Broker themselves.
_Avoid_: radiator fan, booster

**Broker**:
The MQTT broker on the home server that carries all readings.

### Water side

**Flow Temperature**:
Temperature of water leaving a heat source toward the emitters (Heat Pump outlet; radiator inlet).
_Avoid_: outlet temp, supply temp, aanvoer

**Return Temperature**:
Temperature of water coming back from the emitters (Heat Pump inlet; radiator outlet).
_Avoid_: inlet temp, retour

**Flow Rate**:
Volume of water circulating per unit time. Never shorten to "flow", which is reserved for Flow Temperature contexts.
_Avoid_: flow, debiet

**Delta T**:
Flow Temperature minus Return Temperature across a given component.
_Avoid_: spread, temp-delta-io

**DHW**:
Domestic hot water: the tapwater tank heated by the Heat Pump.
_Avoid_: boiler, tapwater, sanitary water

### Air side

**Room Temperature**:
Indoor air temperature measured by the Heat Pump's room sensor; the reference for comfort.
_Avoid_: room air temp, indoor temp

**Booster Ambient Temperature**:
Air temperature measured by a Heat Booster near its radiator.
_Avoid_: temp-ambient

**Outdoor Temperature**:
Outside air temperature measured by the Heat Pump.
_Avoid_: buitentemperatuur, OAT

### Operation

**Operating Mode**:
What the Heat Pump is doing at a moment: Space Heating, DHW, Defrost or Idle.
_Avoid_: state, cycle, energy state

**Space Heating**:
Operating Mode in which the Heat Pump heats the radiator circuit.
_Avoid_: CV, central heating

**Defrost**:
Operating Mode in which the Heat Pump reverses to melt ice off its outdoor coil, briefly taking heat from the water.
_Avoid_: de-ice

**Compressor Frequency**:
Rotational speed of the Heat Pump's inverter compressor in Hz; shows how hard it modulates.
_Avoid_: compressor speed, hz

**Compressor Cycle**:
One continuous period from compressor start to compressor stop.
_Avoid_: run, cycle (alone)

**Short Cycling**:
Compressor Cycles that are too short and too frequent, harming COP and wearing the compressor.

**Heating Curve**:
The Heat Pump's rule mapping outdoor temperature to target Flow Temperature for Space Heating.
_Avoid_: stooklijn, weather curve, auto mode

**Curve Shift**:
A manual offset applied on top of the Heating Curve's target Flow Temperature.
_Avoid_: shift value

### Performance

**COP**:
Coefficient of performance: thermal energy delivered (from the Heat Meter, per-minute from Flow Rate x Delta T calibrated to its counter) divided by electrical energy consumed (from the Electricity Meter) over a given period, reported per Operating Mode and combined.
_Avoid_: efficiency

**Break-even COP**:
The COP at which heat from the Heat Pump costs the same as heat from a gas boiler: all-in electricity price per kWh divided by the gas price per kWh of heat (gas price per m3 / (9.769 kWh/m3 x boiler efficiency)). Above it the Heat Pump is cheaper. Per quarter hour from prices alone, or weighted by actual consumption over a period.
_Avoid_: break-even point, terugverdien-COP

**Calibration Factor**:
Per day, Heat Meter energy counter divided by the energy from Flow Rate x Delta T. Scales the per-minute heat so it agrees with the counter; 1.0 when no valid factor exists.
_Avoid_: correction, fudge factor
