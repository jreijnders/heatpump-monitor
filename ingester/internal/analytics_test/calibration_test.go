package analytics_test

import (
	"context"
	"testing"
	"time"
)

type energyMinute struct {
	heat, uncal, factor *float64
}

func minuteAt(t *testing.T, bucket time.Time) energyMinute {
	t.Helper()
	var m energyMinute
	err := pool.QueryRow(context.Background(),
		"SELECT heat_kwh, heat_kwh_uncalibrated, calibration_factor FROM energy_1m WHERE bucket = $1", bucket).
		Scan(&m.heat, &m.uncal, &m.factor)
	if err != nil {
		t.Fatalf("energy_1m at %v: %v", bucket, err)
	}
	return m
}

func nonNil(t *testing.T, what string, p *float64) float64 {
	t.Helper()
	if p == nil {
		t.Fatalf("%s is NULL", what)
	}
	return *p
}

type calRow struct {
	counter, flowDT, factor *float64
	valid                   bool
	minutes                 int
}

func calibrationDay(t *testing.T, day string) calRow {
	t.Helper()
	var r calRow
	err := pool.QueryRow(context.Background(),
		"SELECT counter_kwh, flow_dt_kwh, factor, valid, minutes FROM heat_calibration_daily WHERE day = $1::date", day).
		Scan(&r.counter, &r.flowDT, &r.factor, &r.valid, &r.minutes)
	if err != nil {
		t.Fatalf("heat_calibration_daily %s: %v", day, err)
	}
	return r
}

// A full Europe/Amsterdam day of Heat Meter polls (60 s) at constant thermal power kw, counter
// advancing scale times the flow x Delta T energy. January is CET: local day d starts at d-1 23:00 UTC.
// Samples start one minute before the day and run 90 min into the next one.
func fullDay(d int, kw, scale float64) []row {
	return heatMeter(at(d-1, 22, 59, 0), at(d+1, 0, 30, 0), time.Minute, 30000, scale, constant(kw))
}

func TestHeatFromFlowTimesDeltaT(t *testing.T) {
	const d = 11
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	// 1000 L/h at 5 K: 1000 * 5 * 1.163 = 5815 W. No counter progress, so no calibration applies.
	var rows []row
	for ts := from; ts.Before(to); ts = ts.Add(time.Minute) {
		rows = append(rows, heatSample(ts, 20000, 1000, 5)...)
	}
	insert(t, rows)
	refresh(t, d)

	var heat, uncal float64
	err := pool.QueryRow(context.Background(),
		"SELECT sum(heat_kwh), sum(heat_kwh_uncalibrated) FROM energy_1m WHERE bucket >= $1 AND bucket < $2",
		at(d, 10, 0, 0), at(d, 11, 0, 0)).Scan(&heat, &uncal)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "uncalibrated kWh/h", uncal, 5.815)
	near(t, "calibrated kWh/h (no factor)", heat, 5.815)
}

func TestHeatNegativeDeltaTCountsAsZero(t *testing.T) {
	const d = 12
	var rows []row
	for i := 0; i < 5; i++ { // return warmer than flow (defrost): meter counter does not decrease
		rows = append(rows, heatSample(at(d, 10, i, 0), 20000, 1000, -4)...)
	}
	insert(t, rows)
	refresh(t, d)

	m := minuteAt(t, at(d, 10, 3, 0))
	near(t, "uncalibrated", nonNil(t, "uncalibrated", m.uncal), 0)
}

func TestHeatCalibrationValidDay(t *testing.T) {
	const d = 14
	// 0.25 kW flow x Delta T = 6.0 kWh/day; counter +6.6 kWh -> factor 1.1.
	insert(t, fullDay(d, 0.25, 1.1))
	// Eight days later: a lone partial day; the factor is older than 7 days -> 1.0.
	insert(t, heatMeter(at(22, 10, 0, 0), at(22, 10, 6, 0), time.Minute, 40000, 1.1, constant(0.25)))
	for _, x := range []int{13, 14, 15, 22} {
		refresh(t, x)
	}

	c := calibrationDay(t, "2026-01-14")
	near(t, "counter_kwh", nonNil(t, "counter", c.counter), 6.6)
	near(t, "flow_dt_kwh", nonNil(t, "flow_dt", c.flowDT), 6.0)
	near(t, "factor", nonNil(t, "factor", c.factor), 1.1)
	if !c.valid {
		t.Error("complete day with enough energy and a plausible factor should be valid")
	}
	if c.minutes != 1440 {
		t.Errorf("minutes = %d, want 1440", c.minutes)
	}

	m := minuteAt(t, at(d, 12, 0, 0))
	near(t, "uncalibrated", nonNil(t, "uncal", m.uncal), 0.25/60)
	near(t, "calibrated", nonNil(t, "heat", m.heat), 0.25/60*1.1)
	near(t, "factor applied", nonNil(t, "factor", m.factor), 1.1)

	// Local day 15 is a 90-minute fragment: not valid itself, uses the factor of day 14.
	if calibrationDay(t, "2026-01-15").valid {
		t.Error("partial day must not be valid")
	}
	m = minuteAt(t, at(15, 0, 10, 0))
	near(t, "fallback factor", nonNil(t, "factor", m.factor), 1.1)
	near(t, "fallback heat", nonNil(t, "heat", m.heat), 0.25/60*1.1)

	m = minuteAt(t, at(22, 10, 3, 0))
	near(t, "expired factor", nonNil(t, "factor", m.factor), 1.0)
	near(t, "expired heat", nonNil(t, "heat", m.heat), nonNil(t, "uncal", m.uncal))
}

func TestHeatCalibrationImplausibleFactorFallsBack(t *testing.T) {
	const d = 24
	insert(t, fullDay(d, 0.25, 2.0)) // counter +12 vs 6.0 flow x Delta T -> factor 2.0
	for _, x := range []int{23, 24, 25} {
		refresh(t, x)
	}

	c := calibrationDay(t, "2026-01-24")
	near(t, "factor", nonNil(t, "factor", c.factor), 2.0)
	if c.valid {
		t.Error("factor 2.0 must not be valid")
	}
	m := minuteAt(t, at(d, 12, 0, 0))
	near(t, "factor applied", nonNil(t, "factor", m.factor), 1.0)
	near(t, "heat", nonNil(t, "heat", m.heat), nonNil(t, "uncal", m.uncal))
}

func TestHeatCalibrationNeedsEnoughEnergy(t *testing.T) {
	const d = 30
	insert(t, fullDay(d, 0.1, 1.1)) // 2.4 kWh/day: below 5 kWh, factor 1.1 would be plausible
	for _, x := range []int{29, 30, 31} {
		refresh(t, x)
	}

	if calibrationDay(t, "2026-01-30").valid {
		t.Error("day with < 5 kWh must not be valid")
	}
	near(t, "factor applied", nonNil(t, "factor", minuteAt(t, at(d, 12, 0, 0)).factor), 1.0)
}

func TestHeatNullWithoutFlowData(t *testing.T) {
	const d = 16
	var rows []row
	for i := 0; i < 5; i++ { // counter only, as before the Gateway published flow and temperatures
		rows = append(rows, row{at(d, 10, i, 0), "heat_meter", "energy_kwh", 20000})
	}
	insert(t, rows)
	refresh(t, d)

	if m := minuteAt(t, at(d, 10, 3, 0)); m.heat != nil || m.uncal != nil {
		t.Errorf("heat = %v / %v, want NULL without flow rate and temperatures", m.heat, m.uncal)
	}
}

// refreshRange materializes the caggs for [from, to).
func refreshRange(t *testing.T, from, to time.Time) {
	t.Helper()
	for _, cagg := range []string{"readings_1m", "readings_1h"} {
		if _, err := pool.Exec(context.Background(),
			"CALL refresh_continuous_aggregate('"+cagg+"', $1::timestamptz, $2::timestamptz)", from, to); err != nil {
			t.Fatal(err)
		}
	}
}

// coverage writes Heat Meter polls spanning exactly `minutes` minutes of intervals from start.
func coverage(start time.Time, minutes int, kw, scale float64) []row {
	return heatMeter(start, start.Add(time.Duration(minutes+1)*time.Minute), time.Minute, 30000, scale, constant(kw))
}

func calibrationMissing(t *testing.T, day string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM heat_calibration_daily WHERE day = $1::date", day).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 0
}

func TestHeatCalibrationFallbackForDayWithoutCalibrationRow(t *testing.T) {
	const d = 40 // Feb 9, local day
	insert(t, fullDay(d, 0.25, 1.2))
	// Later days with decreasing counters (all deltas negative): flow x Delta T exists but the day
	// has no heat_calibration_daily row. Fallback window is the 7 days before (and including) a day.
	for _, k := range []int{3, 7, 8} {
		insert(t, heatMeter(at(d+k, 10, 0, 0), at(d+k, 10, 6, 0), time.Minute, 50000, -1, constant(0.25)))
	}
	refreshRange(t, at(d-1, 0, 0, 0), at(d+10, 0, 0, 0))

	if c := calibrationDay(t, at(d, 12, 0, 0).Format("2006-01-02")); !c.valid {
		t.Fatalf("calibration day should be valid: %+v", c)
	}
	for k, want := range map[int]float64{3: 1.2, 7: 1.2, 8: 1.0} {
		day := at(d+k, 12, 0, 0).Format("2006-01-02")
		if !calibrationMissing(t, day) {
			t.Fatalf("day +%d has a calibration row, test setup is wrong", k)
		}
		m := minuteAt(t, at(d+k, 10, 3, 0))
		near(t, "factor on day +"+string(rune('0'+k)), nonNil(t, "factor", m.factor), want)
		near(t, "heat on day +"+string(rune('0'+k)), nonNil(t, "heat", m.heat), nonNil(t, "uncal", m.uncal)*want)
	}
}

func TestHeatCalibrationNeedsNinetyPercentCoverage(t *testing.T) {
	// Both: 0.3 kW, counter 1.1x, > 5 kWh, plausible factor. Only coverage differs.
	// Local day d starts at d-1 23:00 UTC (CET).
	const low, high = 50, 52
	insert(t, coverage(at(low-1, 22, 59, 0), 1152, 0.3, 1.1)) // 80 % of 1440 min
	insert(t, coverage(at(high-1, 22, 59, 0), 1325, 0.3, 1.1)) // 92 %
	refreshRange(t, at(low-1, 0, 0, 0), at(high+2, 0, 0, 0))

	lowRow := calibrationDay(t, at(low, 12, 0, 0).Format("2006-01-02"))
	if lowRow.valid || lowRow.minutes != 1152 {
		t.Errorf("80%% coverage: valid=%v minutes=%d, want invalid, 1152", lowRow.valid, lowRow.minutes)
	}
	highRow := calibrationDay(t, at(high, 12, 0, 0).Format("2006-01-02"))
	if !highRow.valid || highRow.minutes != 1325 {
		t.Errorf("92%% coverage: valid=%v minutes=%d, want valid, 1325", highRow.valid, highRow.minutes)
	}
}

func TestHeatCalibrationCoverageOnDSTDays(t *testing.T) {
	ams, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 23 h day (Mar 29 2026): 1260 min = 91 % of 1380 (87 % of 24 h) -> valid.
	// 25 h day (Oct 25 2026): 1340 min = 89 % of 1500 (93 % of 24 h) -> invalid.
	short := time.Date(2026, 3, 29, 0, 0, 0, 0, ams).Add(-time.Minute)
	long := time.Date(2026, 10, 25, 0, 0, 0, 0, ams).Add(-time.Minute)
	insert(t, coverage(short, 1260, 0.3, 1.1))
	insert(t, coverage(long, 1340, 0.3, 1.1))
	refreshRange(t, short.Add(-time.Hour), short.Add(26*time.Hour))
	refreshRange(t, long.Add(-time.Hour), long.Add(27*time.Hour))

	if r := calibrationDay(t, "2026-03-29"); !r.valid || r.minutes != 1260 {
		t.Errorf("23 h day: valid=%v minutes=%d, want valid, 1260", r.valid, r.minutes)
	}
	if r := calibrationDay(t, "2026-10-25"); r.valid || r.minutes != 1340 {
		t.Errorf("25 h day: valid=%v minutes=%d, want invalid, 1340", r.valid, r.minutes)
	}
}
