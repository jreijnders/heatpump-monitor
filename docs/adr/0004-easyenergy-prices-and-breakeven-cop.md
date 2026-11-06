# easyEnergy prices for cost and Break-even COP

Electricity and gas prices of the household's easyEnergy dynamic contract are fetched from easyEnergy's public price-graph API (`https://price-graph.mijn.easyenergy.com/api/prices`) by a small `prices` service (same Go module and image as the ingester, `cmd/prices`) into the `energy_prices` table. Electricity at quarter-hour, gas per day. The `invoicePrice` field (market price incl. VAT + energy tax + easyEnergy purchase fee) is the per-unit price used everywhere. Break-even COP and the "same heat from gas" cost compare against a gas boiler with 90 % efficiency on the gross calorific value (9.769 kWh/m3), both set in SQL functions (`gas_boiler_efficiency()`, `gas_kwh_per_m3()`).

## Considered options

- **Grafana Infinity/JSON datasource straight to the API**: no storage, but prices cannot be joined with per-minute energy, so no cost or consumption-weighted Break-even COP.
- **Fetch in the ingester process**: fewer containers, but mixes an HTTP poller into the MQTT pipeline; a failing API would share its lifecycle.
- **Separate `prices` service writing to TimescaleDB** (chosen): prices join `energy_1m` per quarter hour; the ingester stays MQTT-only.

## Decision details

- API times are Europe/Amsterdam wall-clock without offset. On the autumn DST day the repeated hour is published once; its minutes have no price and drop out of the cost views. The spring gap hour is simply absent.
- Startup backfills `PRICES_BACKFILL_DAYS` (default 60); then hourly refresh of two days back to tomorrow (published ~13:00). Rows are upserted, so revisions overwrite.
- Only per-unit prices count. Fixed charges (standing charges, network costs, energy tax credit) do not depend on consumption and are left out; the comparison is marginal, i.e. assumes the gas connection stays.
- Break-even COP is reported two ways: per quarter hour from prices alone (`breakeven_cop`), and consumption-weighted over a period (`energy_cost_daily`, dashboard stats), which is the one to compare with the period's COP: COP > weighted Break-even COP exactly when the electricity cost is below the gas cost.

## Consequences

- Cost results depend on the assumed boiler efficiency; 0.85-0.95 moves the Break-even COP by about ±6 %.
- Energy tax applies per tier; the API reports the first-tier rate, correct for a household (electricity < 10 000 kWh/yr).
- An easyEnergy API change stops price updates; the dashboard then shows gaps, not wrong values. The ingester is unaffected.
