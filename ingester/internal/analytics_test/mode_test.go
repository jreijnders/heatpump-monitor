package analytics_test

import (
	"context"
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
