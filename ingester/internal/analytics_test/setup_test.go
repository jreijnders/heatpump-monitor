package analytics_test

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test uses its own UTC day so fixtures never overlap.

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		os.Exit(0) // skip: no database
	}
	var err error
	pool, err = pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func day(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }

func at(d, h, m, s int) time.Time { return time.Date(2026, 1, d, h, m, s, 0, time.UTC) }

type row struct {
	t      time.Time
	device string
	metric string
	value  float64
}

func insert(t *testing.T, rows []row) {
	t.Helper()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO readings (time, device, metric, value) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			r.t, r.device, r.metric, r.value)
	}
	res := pool.SendBatch(context.Background(), b)
	defer res.Close()
	for range rows {
		if _, err := res.Exec(); err != nil {
			t.Fatal(err)
		}
	}
}

// refresh materializes both caggs for a whole UTC day.
func refresh(t *testing.T, d int) {
	t.Helper()
	from, to := day(d).Format(time.RFC3339), day(d+1).Format(time.RFC3339)
	for _, cagg := range []string{"readings_1m", "readings_1h"} {
		if _, err := pool.Exec(context.Background(),
			"CALL refresh_continuous_aggregate('"+cagg+"', '"+from+"', '"+to+"')"); err != nil {
			t.Fatal(err)
		}
	}
}

// flags describes the heat_pump status flags for one minute.
type flags struct{ comp, defrost, dhw float64 }

// heatPumpFlags writes 10 s flag samples for every minute in [from, to); modeAt picks the flags per minute.
func heatPumpFlags(from, to time.Time, modeAt func(min time.Time) flags) []row {
	var rows []row
	for ts := from; ts.Before(to); ts = ts.Add(10 * time.Second) {
		f := modeAt(ts.Truncate(time.Minute))
		rows = append(rows,
			row{ts, "heat_pump", "compressor_on", f.comp},
			row{ts, "heat_pump", "defrost_active", f.defrost},
			row{ts, "heat_pump", "dhw_active", f.dhw})
	}
	return rows
}

// counter writes a cumulative counter sampled every step in [from, to). The increment added at a
// sample is rate(minute of sample) kWh/h * step.
func counter(device string, from, to time.Time, step time.Duration, base float64, rate func(min time.Time) float64) []row {
	var rows []row
	v := base
	for ts := from; ts.Before(to); ts = ts.Add(step) {
		v += rate(ts.Truncate(time.Minute)) * step.Hours()
		rows = append(rows, row{ts, device, "energy_kwh", v})
	}
	return rows
}

func constant(x float64) func(time.Time) float64 { return func(time.Time) float64 { return x } }

var (
	spaceHeating = flags{comp: 1}
	idle         = flags{}
)

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// whPerLitreKelvin must match the constant in db/schema/030_cop.sql.
const whPerLitreKelvin = 1.163

// heatSample writes one Heat Meter poll: flow rate (L/h), flow/return temperatures and the counter.
func heatSample(ts time.Time, counterKWh, flowLph, deltaT float64) []row {
	const ret = 30.0
	return []row{
		{ts, "heat_meter", "energy_kwh", counterKWh},
		{ts, "heat_meter", "flow_rate", flowLph},
		{ts, "heat_meter", "flow_temp", ret + deltaT},
		{ts, "heat_meter", "return_temp", ret},
	}
}

// heatMeter writes Heat Meter polls every step in [from, to) at 1000 L/h with the Delta T that
// gives thermal power kw(minute of sample) (kW). The counter advances by scale * kw * step.
func heatMeter(from, to time.Time, step time.Duration, base, scale float64, kw func(min time.Time) float64) []row {
	var rows []row
	v := base
	for ts := from; ts.Before(to); ts = ts.Add(step) {
		p := kw(ts.Truncate(time.Minute))
		v += scale * p * step.Hours()
		// P[W] = 1000 L/h * dT * 1.163  =>  dT = kw*1000 / (1000 * 1.163) = kw / 1.163
		rows = append(rows, heatSample(ts, v, 1000, p/whPerLitreKelvin)...)
	}
	return rows
}
