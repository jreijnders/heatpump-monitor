package analytics_test

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestOperatingMode(t *testing.T) {
	// Day 1: one minute per flag combination, 6 samples each.
	cases := []struct {
		name string
		f    flags
		want string
	}{
		{"idle", flags{0, 0, 0}, "Idle"},
		{"space heating", flags{1, 0, 0}, "Space Heating"},
		{"dhw", flags{1, 0, 1}, "DHW"},
		{"dhw flag without compressor", flags{0, 0, 1}, "Idle"},
		{"defrost", flags{1, 1, 0}, "Defrost"},
		{"defrost beats dhw", flags{1, 1, 1}, "Defrost"},
		{"defrost without compressor", flags{0, 1, 0}, "Defrost"},
	}
	var rows []row
	for i, c := range cases {
		start := at(1, 10, i, 0)
		rows = append(rows, heatPumpFlags(start, start.Add(time.Minute), func(time.Time) flags { return c.f })...)
	}
	// Majority per minute: 4 of 6 samples on -> Space Heating; 2 of 6 -> Idle.
	maj := func(onSamples int, minute int) {
		for i := 0; i < 6; i++ {
			v := 0.0
			if i < onSamples {
				v = 1
			}
			rows = append(rows, row{at(1, 11, minute, i*10), "heat_pump", "compressor_on", v})
		}
	}
	maj(4, 0)
	maj(2, 1)
	insert(t, rows)
	refresh(t, 1)

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			err := pool.QueryRow(context.Background(),
				"SELECT operating_mode FROM heat_pump_1m WHERE bucket = $1", at(1, 10, i, 0)).Scan(&got)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("mode = %q, want %q", got, c.want)
			}
		})
	}
	for minute, want := range map[int]string{0: "Space Heating", 1: "Idle"} {
		var got string
		if err := pool.QueryRow(context.Background(),
			"SELECT operating_mode FROM heat_pump_1m WHERE bucket = $1", at(1, 11, minute, 0)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("majority minute %d: mode = %q, want %q", minute, got, want)
		}
	}
}

func TestOperatingModeDefrostWithoutCompressorSample(t *testing.T) {
	const d = 10
	var rows []row
	for i := 0; i < 6; i++ { // 10:00: defrost flag only, no compressor_on sample
		rows = append(rows, row{at(d, 10, 0, i*10), "heat_pump", "defrost_active", 1})
	}
	for i := 0; i < 6; i++ { // 10:01: neither flag, only an unrelated metric
		rows = append(rows, row{at(d, 10, 1, i*10), "heat_pump", "flow_temp", 30})
	}
	insert(t, rows)
	refresh(t, d)

	for minute, want := range map[int]*string{0: ptr("Defrost"), 1: nil} {
		var got *string
		if err := pool.QueryRow(context.Background(),
			"SELECT operating_mode FROM heat_pump_1m WHERE bucket = $1", at(d, 10, minute, 0)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("minute %d: mode = %v, want %v", minute, got, want)
		}
	}
}

func ptr(s string) *string { return &s }

func columnNames(t *testing.T, view string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT column_name FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position", view)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// Views are deployed: existing columns must keep name and position (CREATE OR REPLACE VIEW only appends).
func TestViewColumnsAppendOnly(t *testing.T) {
	prefix := map[string][]string{
		"heat_pump_1m": {"bucket", "flow_temp", "return_temp", "outdoor_temp", "room_temp", "dhw_temp",
			"target_flow_temp", "flow_rate", "curve_shift", "compressor_on", "defrost_active", "dhw_active",
			"water_pump_on", "error_code", "operating_mode"},
		"energy_1m": {"bucket", "operating_mode", "heat_kwh", "elec_kwh"},
		"cop_hourly": {"bucket", "operating_mode", "heat_kwh", "elec_kwh", "cop", "minutes"},
		"cop_daily":  {"bucket", "operating_mode", "heat_kwh", "elec_kwh", "cop", "minutes"},
	}
	for view, want := range prefix {
		got := columnNames(t, view)
		if len(got) < len(want) {
			t.Fatalf("%s: columns %v", view, got)
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("%s column %d = %q, want %q", view, i+1, got[i], w)
			}
		}
	}
}

// Delta T across the Heat Pump = Flow Temperature (outlet) - Return Temperature (inlet), per minute.
func TestHeatPumpDeltaT(t *testing.T) {
	const d = 18
	var rows []row
	for i := 0; i < 6; i++ {
		rows = append(rows,
			row{at(d, 10, 0, i*10), "heat_pump", "flow_temp", 40.5},
			row{at(d, 10, 0, i*10), "heat_pump", "return_temp", 35.0},
		)
	}
	rows = append(rows, row{at(d, 10, 1, 0), "heat_pump", "flow_temp", 41}) // no return_temp: NULL
	insert(t, rows)
	refresh(t, d)

	for minute, want := range map[int]*float64{0: fptr(5.5), 1: nil} {
		var got *float64
		if err := pool.QueryRow(context.Background(),
			"SELECT delta_t FROM heat_pump_1m WHERE bucket = $1", at(d, 10, minute, 0)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (want == nil) || (got != nil && math.Abs(*got-*want) > 1e-9) {
			t.Errorf("minute %d: delta_t = %v, want %v", minute, got, want)
		}
	}
}

func fptr(f float64) *float64 { return &f }

func TestHeatPumpExtraMetrics(t *testing.T) {
	const d = 13
	var rows []row
	for i := 0; i < 6; i++ {
		ts := at(d, 10, 0, i*10)
		rows = append(rows,
			row{ts, "heat_pump", "compressor_on", 1},
			row{ts, "heat_pump", "compressor_hz", float64(30 + 2*i)},
			row{ts, "heat_pump", "discharge_temp", 70},
			row{ts, "heat_pump", "high_pressure", 20},
			row{ts, "heat_pump", "dhw_heater_on", float64(i % 2)},
			row{ts, "heat_pump", "error_active", 0})
	}
	insert(t, rows)
	refresh(t, d)

	var hz, discharge, hp, heater, errAct *float64
	var mode string
	err := pool.QueryRow(context.Background(),
		"SELECT compressor_hz, discharge_temp, high_pressure, dhw_heater_on, error_active, operating_mode FROM heat_pump_1m WHERE bucket = $1",
		at(d, 10, 0, 0)).Scan(&hz, &discharge, &hp, &heater, &errAct, &mode)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "compressor_hz", nonNil(t, "hz", hz), 35)
	near(t, "discharge_temp", nonNil(t, "discharge", discharge), 70)
	near(t, "high_pressure", nonNil(t, "hp", hp), 20)
	near(t, "dhw_heater_on", nonNil(t, "heater", heater), 0.5)
	near(t, "error_active", nonNil(t, "err", errAct), 0)
	if mode != "Space Heating" {
		t.Errorf("mode = %q", mode)
	}
	var unset *float64
	if err := pool.QueryRow(context.Background(),
		"SELECT low_pressure FROM heat_pump_1m WHERE bucket = $1", at(d, 10, 0, 0)).Scan(&unset); err != nil || unset != nil {
		t.Errorf("low_pressure = %v err=%v, want NULL", unset, err)
	}
}
