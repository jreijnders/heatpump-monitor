package analytics_test

import (
	"context"
	"math"
	"testing"
	"time"
)

type copRow struct {
	heat, elec float64
	cop        *float64
	minutes    int
}

func copByMode(t *testing.T, view string, bucket time.Time) map[string]copRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT operating_mode, heat_kwh, elec_kwh, cop, minutes FROM "+view+" WHERE bucket = $1", bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]copRow{}
	for rows.Next() {
		var mode string
		var r copRow
		if err := rows.Scan(&mode, &r.heat, &r.elec, &r.cop, &r.minutes); err != nil {
			t.Fatal(err)
		}
		out[mode] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCOPSpaceHeatingHour(t *testing.T) {
	const d = 2
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 5000, constant(0.75))...)
	rows = append(rows, heatMeter(from, to, time.Minute, 20000, 1, constant(3))...)
	insert(t, rows)
	refresh(t, d)

	got := copByMode(t, "cop_hourly", at(d, 10, 0, 0))
	sh := got["Space Heating"]
	near(t, "heat_kwh", sh.heat, 3.0)
	near(t, "elec_kwh", sh.elec, 0.75)
	if sh.cop == nil {
		t.Fatal("cop is NULL")
	}
	near(t, "cop", *sh.cop, 4.0)
	if sh.minutes != 60 {
		t.Errorf("minutes = %d, want 60", sh.minutes)
	}
	if _, ok := got["Combined"]; !ok {
		t.Error("missing Combined row")
	}
	if len(got) != 2 {
		t.Errorf("rows = %v, want Space Heating + Combined only", got)
	}
}

func TestCOPPerModeAndCombined(t *testing.T) {
	const d = 3
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	dhwEnd := at(d, 10, 30, 0)
	inDHW := func(m time.Time) bool { return !m.Before(at(d, 10, 0, 0)) && m.Before(dhwEnd) }
	rows := heatPumpFlags(from, to, func(m time.Time) flags {
		if inDHW(m) {
			return flags{comp: 1, dhw: 1}
		}
		return spaceHeating
	})
	elec := func(m time.Time) float64 { // kWh/h
		if inDHW(m) {
			return 1.0
		}
		return 0.5
	}
	heat := func(m time.Time) float64 {
		if inDHW(m) {
			return 2.0
		}
		return 3.0
	}
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 100, elec)...)
	rows = append(rows, heatMeter(from, to, time.Minute, 300, 1, heat)...)
	insert(t, rows)
	refresh(t, d)

	got := copByMode(t, "cop_hourly", at(d, 10, 0, 0))
	// Heat per minute is the trapezoid of two 60 s samples, so the minute of each power step gets the
	// mean of both powers. DHW 10:00-10:29: 2.5 + 29*2 = 60.5 kW-min; SH 10:30-10:59: 2.5 + 29*3 = 89.5.
	// Elec: DHW 0.5 kWh, SH 0.25 kWh. Combined heat 150 kW-min = 2.5 kWh.
	cases := map[string][3]float64{
		"DHW":           {60.5 / 60, 0.5, 60.5 / 60 / 0.5},
		"Space Heating": {89.5 / 60, 0.25, 89.5 / 60 / 0.25},
		"Combined":      {2.5, 0.75, 2.5 / 0.75},
	}
	for mode, want := range cases {
		r, ok := got[mode]
		if !ok || r.cop == nil {
			t.Errorf("%s: missing row or NULL cop: %+v", mode, r)
			continue
		}
		near(t, mode+" heat", r.heat, want[0])
		near(t, mode+" elec", r.elec, want[1])
		near(t, mode+" cop", *r.cop, want[2])
	}

	// Daily: Amsterdam day boundaries (CET, UTC+1 in January): day d starts d-1 23:00 UTC.
	daily := copByMode(t, "cop_daily", at(d-1, 23, 0, 0))
	if r, ok := daily["Combined"]; !ok || r.cop == nil {
		t.Fatalf("cop_daily Combined missing: %+v", daily)
	} else if r.heat < 4 {
		t.Errorf("cop_daily Combined heat = %v, want >= 4", r.heat)
	}
}

func TestCOPExcludesResetAndGap(t *testing.T) {
	const d = 4
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })

	// Electricity counter resets to ~0 at 10:20:00 (minute 10:20 has a negative delta).
	elec := counter("electricity_meter", from, to, 10*time.Second, 5000, constant(1))
	resetAt := at(d, 10, 20, 0)
	var reset float64
	for i := range elec {
		if elec[i].t.Before(resetAt) {
			continue
		}
		if reset == 0 {
			reset = elec[i].value - 1.0/360 // keep the first post-reset increment
		}
		elec[i].value -= reset
	}
	rows = append(rows, elec...)

	// Heat meter silent 10:40-10:49 inclusive: no samples, so minutes 10:40..10:49 have no heat delta
	// and minute 10:50 has a 11-minute-old previous sample.
	for _, r := range heatMeter(from, to, time.Minute, 20000, 1, constant(3)) {
		if !r.t.Before(at(d, 10, 40, 0)) && r.t.Before(at(d, 10, 50, 0)) {
			continue
		}
		rows = append(rows, r)
	}
	insert(t, rows)
	refresh(t, d)

	got := copByMode(t, "cop_hourly", at(d, 10, 0, 0))
	c := got["Combined"]
	// Excluded minutes: 10:20 (reset) + 10:40..10:49 (no heat) + 10:50 (gap) = 12 -> 48 counted.
	if c.minutes != 48 {
		t.Errorf("minutes = %d, want 48", c.minutes)
	}
	near(t, "elec", c.elec, 48.0/60)
	near(t, "heat", c.heat, 48.0*3/60)
	if c.cop == nil {
		t.Fatal("cop NULL")
	}
	near(t, "cop", *c.cop, 3.0)
}

func TestCOPNoDivisionByZero(t *testing.T) {
	const d = 5
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 100, constant(0))...)
	rows = append(rows, heatMeter(from, to, time.Minute, 300, 1, constant(1))...)
	insert(t, rows)
	refresh(t, d)

	got := copByMode(t, "cop_hourly", at(d, 10, 0, 0))
	if c, ok := got["Combined"]; !ok || c.cop != nil {
		t.Errorf("Combined = %+v (ok=%v), want a row with NULL cop", c, ok)
	}
}

func TestHeatMeterEnergyAttributedToRightMinute(t *testing.T) {
	const d = 6
	from, to := at(d, 10, 0, 0), at(d, 10, 15, 0)
	// DHW only in minute 10:05. Heat Meter polls at :30 each minute; thermal power is 0 except a
	// 6 kW sample at 10:05:30. Trapezoid: the interval (10:04:30, 10:05:30] holds 3 kW * 1 min =
	// 0.05 kWh -> minute 10:05 (DHW); (10:05:30, 10:06:30] holds 0.05 kWh -> minute 10:06 (Space Heating).
	rows := heatPumpFlags(from, to, func(m time.Time) flags {
		if m.Equal(at(d, 10, 5, 0)) {
			return flags{comp: 1, dhw: 1}
		}
		return spaceHeating
	})
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 100, constant(0.6))...)
	rows = append(rows, heatMeter(at(d, 10, 0, 30), to, time.Minute, 50, 1, func(m time.Time) float64 {
		if m.Equal(at(d, 10, 5, 0)) {
			return 6
		}
		return 0
	})...)
	insert(t, rows)
	refresh(t, d)

	q, err := pool.Query(context.Background(),
		"SELECT bucket, operating_mode, heat_kwh FROM energy_1m WHERE bucket >= $1 AND bucket < $2 AND heat_kwh IS NOT NULL ORDER BY bucket",
		from, to)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	n := 0
	for q.Next() {
		var b time.Time
		var mode string
		var heat float64
		if err := q.Scan(&b, &mode, &heat); err != nil {
			t.Fatal(err)
		}
		n++
		switch {
		case b.Equal(at(d, 10, 5, 0)):
			near(t, "heat at 10:05", heat, 0.05)
			if mode != "DHW" {
				t.Errorf("mode at 10:05 = %q, want DHW", mode)
			}
		case b.Equal(at(d, 10, 6, 0)):
			near(t, "heat at 10:06", heat, 0.05)
			if mode != "Space Heating" {
				t.Errorf("mode at 10:06 = %q, want Space Heating", mode)
			}
		default:
			near(t, "heat at "+b.Format("15:04"), heat, 0)
		}
	}
	if err := q.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 14 { // 10:01..10:14 (10:00 has no previous sample)
		t.Errorf("minutes with heat = %d, want 14", n)
	}
}

// heatMeterTicks writes one Heat Meter sample per minute in [from, to) (3 kW each) at the time
// given by sampleAt(minute), skipping minutes where sampleAt returns ok=false.
func heatMeterTicks(from, to time.Time, sampleAt func(min time.Time) (time.Time, bool)) []row {
	var rows []row
	v := 20000.0
	for m := from; m.Before(to); m = m.Add(time.Minute) {
		v += 0.05
		if ts, ok := sampleAt(m); ok {
			rows = append(rows, heatSample(ts, v, 1000, 3/whPerLitreKelvin)...)
		}
	}
	return rows
}

func combinedHour(t *testing.T, d int) copRow {
	t.Helper()
	c, ok := copByMode(t, "cop_hourly", at(d, 10, 0, 0))["Combined"]
	if !ok || c.cop == nil {
		t.Fatalf("Combined missing or NULL cop: %+v", c)
	}
	return c
}

func TestCOPMissedHeatPollNotOverstated(t *testing.T) {
	const d = 8
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 5000, constant(0.75))...)
	// The 10:30 poll is missed: heat delta at 10:31 would span 2 min against a 1 min elec delta.
	rows = append(rows, heatMeterTicks(from, to, func(m time.Time) (time.Time, bool) {
		return m.Add(30 * time.Second), !m.Equal(at(d, 10, 30, 0))
	})...)
	insert(t, rows)
	refresh(t, d)

	c := combinedHour(t, d)
	if c.minutes != 58 { // 10:30 (no heat sample) and 10:31 (2-minute heat delta) excluded
		t.Errorf("minutes = %d, want 58", c.minutes)
	}
	near(t, "cop", *c.cop, 4.0)
}

func TestCOPJitteredPollAcrossMinuteBoundaryCounts(t *testing.T) {
	const d = 9
	from, to := at(d, 9, 0, 0), at(d, 12, 0, 0)
	rows := heatPumpFlags(from, to, func(time.Time) flags { return spaceHeating })
	rows = append(rows, counter("electricity_meter", from, to, 10*time.Second, 5000, constant(0.75))...)
	// Polls drift from just before to just after the minute boundary: samples at 10:29:59.9 and
	// 10:31:00.1 (60.2 s apart) leave bucket 10:30 empty but are one poll interval apart.
	rows = append(rows, heatMeterTicks(from, to, func(m time.Time) (time.Time, bool) {
		if m.Before(at(d, 10, 31, 0)) {
			return m.Add(-100 * time.Millisecond), true
		}
		return m.Add(100 * time.Millisecond), true
	})...)
	insert(t, rows)
	refresh(t, d)

	c := combinedHour(t, d)
	if c.minutes != 59 { // only the empty 10:30 bucket is excluded
		t.Errorf("minutes = %d, want 59", c.minutes)
	}
	// The 60.2 s interval carries 0.2 s more heat than the 60 s of electricity it is paired with.
	if math.Abs(*c.cop-4.0) > 1e-2 {
		t.Errorf("cop = %v, want ~4.0", *c.cop)
	}
}
