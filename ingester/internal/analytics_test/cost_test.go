package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Must match db/schema/060_energy_prices.sql.
const (
	gasKWhPerM3        = 9.769
	gasBoilerEfficency = 0.963
)

func insertPrice(t *testing.T, b *pgx.Batch, commodity string, starts, ends time.Time, invoice float64) {
	t.Helper()
	b.Queue(`INSERT INTO energy_prices (commodity, starts, ends, unit, market_price_ex_vat, market_price_inc_vat,
	                                    energy_tax, purchase_fee, invoice_price)
	         VALUES ($1, $2, $3, 'x', 0, 0, 0, 0, $4)
	         ON CONFLICT (commodity, starts) DO UPDATE SET invoice_price = EXCLUDED.invoice_price`,
		commodity, starts, ends, invoice)
}

func TestEnergyCostAndBreakevenCOP(t *testing.T) {
	const d = 70 // Mar 11 (CET): > 7 days from other heat fixtures, so the calibration factor is 1.0
	ctx := context.Background()
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 7000, constant(0.75))...)
	rows = append(rows, heatMeter(from, to, time.Minute, 30000, 1, constant(3))...)
	insert(t, rows)
	refresh(t, d)

	// Electricity 0.30 EUR/kWh, 0.50 in the 10:00 quarter; no price for the 11:45 quarter.
	// Gas 1.80 EUR/m3 for the local day (starts 23:00 UTC the day before).
	const elecPrice, peakPrice, gasPrice = 0.30, 0.50, 1.80
	b := &pgx.Batch{}
	for q := from; q.Before(at(d, 11, 45, 0)); q = q.Add(15 * time.Minute) {
		p := elecPrice
		if q.Equal(at(d, 10, 0, 0)) {
			p = peakPrice
		}
		insertPrice(t, b, "electricity", q, q.Add(15*time.Minute), p)
	}
	localMidnight := day(d).Add(-time.Hour)
	insertPrice(t, b, "gas", localMidnight, localMidnight.Add(24*time.Hour), gasPrice)
	if err := pool.SendBatch(ctx, b).Close(); err != nil {
		t.Fatal(err)
	}

	gasHeat := gasPrice / (gasKWhPerM3 * gasBoilerEfficency) // EUR per kWh heat from gas
	const heatMin, elecMin = 3.0 / 60, 0.75 / 60             // kWh per minute

	t.Run("breakeven_cop", func(t *testing.T) {
		var be, gh float64
		if err := pool.QueryRow(ctx, `SELECT breakeven_cop, gas_heat_price_eur_kwh FROM breakeven_cop WHERE starts = $1`,
			at(d, 10, 0, 0)).Scan(&be, &gh); err != nil {
			t.Fatal(err)
		}
		near(t, "gas_heat_price_eur_kwh", gh, gasHeat)
		near(t, "breakeven_cop", be, peakPrice/gasHeat)
	})

	t.Run("energy_cost_1m", func(t *testing.T) {
		var elecCost, gasCost, be float64
		if err := pool.QueryRow(ctx, `SELECT elec_cost_eur, gas_cost_eur, breakeven_cop FROM energy_cost_1m WHERE bucket = $1`,
			at(d, 10, 5, 0)).Scan(&elecCost, &gasCost, &be); err != nil {
			t.Fatal(err)
		}
		near(t, "elec_cost_eur", elecCost, elecMin*peakPrice)
		near(t, "gas_cost_eur", gasCost, heatMin*gasHeat)
		near(t, "breakeven_cop", be, peakPrice/gasHeat)

		var unpriced int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM energy_cost_1m WHERE bucket >= $1 AND bucket < $2`,
			at(d, 11, 45, 0), to).Scan(&unpriced); err != nil {
			t.Fatal(err)
		}
		if unpriced != 0 {
			t.Errorf("minutes without a price = %d, want 0 (absent)", unpriced)
		}
	})

	t.Run("energy_cost_daily", func(t *testing.T) {
		var heat, elec, cop, be, elecCost, gasCost, savings float64
		var minutes int
		if err := pool.QueryRow(ctx, `
			SELECT heat_kwh, elec_kwh, cop, breakeven_cop, elec_cost_eur, gas_cost_eur, savings_eur, minutes
			FROM energy_cost_daily WHERE bucket = $1 AND operating_mode = 'Combined'`, localMidnight).
			Scan(&heat, &elec, &cop, &be, &elecCost, &gasCost, &savings, &minutes); err != nil {
			t.Fatal(err)
		}
		// 09:01..11:44 (09:00 has no previous sample; 11:45 quarter unpriced).
		if minutes != 164 {
			t.Errorf("minutes = %d, want 164", minutes)
		}
		m := float64(minutes)
		wantElecCost := m*elecMin*elecPrice + 15*elecMin*(peakPrice-elecPrice)
		near(t, "cop", cop, 4)
		near(t, "elec_cost_eur", elecCost, wantElecCost)
		near(t, "gas_cost_eur", gasCost, m*heatMin*gasHeat)
		near(t, "savings_eur", savings, m*heatMin*gasHeat-wantElecCost)
		near(t, "breakeven_cop (weighted)", be, (wantElecCost/(m*elecMin))/gasHeat)
		if (cop > be) != (savings > 0) {
			t.Errorf("cop %v vs breakeven %v disagrees with savings %v", cop, be, savings)
		}
	})
}
