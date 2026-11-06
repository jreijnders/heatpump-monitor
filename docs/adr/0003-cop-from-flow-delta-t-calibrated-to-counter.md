# COP from Heat Meter flow x Delta T, calibrated to its energy counter

Heat delivered per minute is computed from the Heat Meter's Flow Rate and Delta T (P[W] = flow rate [L/h] x (Flow Temperature - Return Temperature) [K] x 1.163, trapezoid between its 60 s samples). Per Europe/Amsterdam day that energy is scaled by a Calibration Factor so it matches the Heat Meter's own energy counter. Electricity is still taken from Electricity Meter counter deltas. COP = Σ calibrated heat / Σ electricity, per Operating Mode and combined (`energy_1m`, `cop_hourly`, `cop_daily`).

## Context

The original decision (counter deltas for both meters) assumed the Kamstrup Multical reports energy in Wh. Over M-Bus it reports whole kWh (record "Energy (kWh)", e.g. 44584). A kWh step is 1/3 to 1/6 of an hour of typical output, so per-minute and per-Operating-Mode COP from counter steps is meaningless (a DHW run would be a handful of steps). The Heat Meter is battery powered and polled every 60 s, not 10 s.

Heat Meter readings every 60 s: energy (1 kWh), flow rate (1 L/h), Flow and Return Temperature (0.01 C), power (100 W), volume (0.01 m3). Electricity Meter every 10 s: energy (~0.01 kWh), power and electrical values.

## Considered options

- **Counter deltas only** (previous decision): exact in total and robust to missed polls, but 1 kWh quantisation. Fine over a day, unusable per minute or per mode.
- **Integrate the meter's power_w**: 100 W resolution is ~5 % of the typical 2 kW, and ~20 % at 500 W (modulating at low load). Poor, and biased by rounding.
- **Flow x Delta T, uncalibrated**: resolution is good (0.01 K, 1 L/h) and gives per-minute heat. But it carries systematic error: water constant approximation (1.163 vs ~1.15 at heating temperatures), flow rate and temperature offsets. Totals would not agree with the Heat Meter's invoice-grade counter.
- **Flow x Delta T calibrated per day to the counter** (chosen): per-minute shape from flow x Delta T, absolute level from the counter, whose error over a day is at most 1 kWh.

## Decision details

- Interval energy: trapezoid of the power at both samples times the real interval, attributed to the minute of the later sample. A power step is split over the two neighbouring minutes rather than lagging one minute. Negative Delta T (Defrost) counts as 0 W, as the meter counter does not decrease (assumption). Excluded (NULL) when the gap between samples exceeds `max_counter_gap('heat_meter')` or an input is missing.
- Calibration Factor per local day = Σ counter deltas / Σ flow x Delta T energy, over intervals where both are known (`heat_calibration_daily`). Valid when covered time >= 90 % of the day, both sums >= 5 kWh, and 0.8 <= factor <= 1.25. A factor outside that range indicates a sensor or meter fault, not calibration: it is shown but not applied.
- Minutes on a day without a valid factor use the most recent valid factor of the previous 7 days, else 1.0. The current day is therefore always provisional. Factors are computed once per day in the view; at home scale a full scan per query is acceptable.
- `energy_1m.heat_kwh` is calibrated; `heat_kwh_uncalibrated` and `calibration_factor` stay visible. Columns of the deployed views are only appended.

## Consequences

- Per-minute and per-mode COP is usable. Daily totals match the Heat Meter counter on calibrated days (within the day-edge and 1 kWh quantisation error).
- Per-mode heat is as accurate as flow x Delta T (relative shape), absolutely scaled by one factor per day: not per-mode corrected.
- Calibration Factor drift over weeks (visible in `heat_calibration_daily` and the dashboard) flags sensor or flow-rate problems.
- Needs flow rate and both temperatures in every Heat Meter sample; data from before the Gateway published them has no heat (NULL), so COP is absent for that period.
- Slower Heat Meter polling needs `max_counter_gap('heat_meter')` raised; per-minute resolution is limited by the 60 s poll.
- Assumption: the Electricity Meter measures the whole Heat Pump including its electric heaters (DHW Heater, backup heaters). Electricity then includes resistive heating, with COP near 1 for that share. Check `dhw_heater_on` / `backup_heater_*_on` in `heat_pump_1m` before reading a low COP as a Heat Pump problem.
