package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type cycle struct {
	start time.Time
	end   *time.Time
	dur   *time.Duration
}

func cycles(t *testing.T, from, to time.Time) []cycle {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT cycle_start, cycle_end, duration FROM compressor_cycles($1, $2)", from, to)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []cycle
	for rows.Next() {
		var c cycle
		var dur pgxInterval
		if err := rows.Scan(&c.start, &c.end, &dur); err != nil {
			t.Fatal(err)
		}
		if dur.Valid {
			d := time.Duration(dur.Microseconds) * time.Microsecond
			c.dur = &d
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCompressorCycles(t *testing.T) {
	const d = 30 // look-back reads 24h before the range: keep clear of other fixtures
	// 10 s samples. On: 10:00:00-10:03:50 and 10:20:00-10:59:50. Off otherwise.
	// cycle_start = first on sample, cycle_end = first off sample -> 4 min and 40 min exactly.
	on := func(ts time.Time) bool {
		return (!ts.Before(at(d, 10, 0, 0)) && ts.Before(at(d, 10, 4, 0))) ||
			(!ts.Before(at(d, 10, 20, 0)) && ts.Before(at(d, 11, 0, 0)))
	}
	var rows []row
	for ts := at(d, 9, 50, 0); ts.Before(at(d, 11, 10, 0)); ts = ts.Add(10 * time.Second) {
		v := 0.0
		if on(ts) {
			v = 1
		}
		rows = append(rows, row{ts, "heat_pump", "compressor_on", v})
	}
	insert(t, rows)

	t.Run("two cycles", func(t *testing.T) {
		got := cycles(t, at(d, 9, 0, 0), at(d, 12, 0, 0))
		if len(got) != 2 {
			t.Fatalf("cycles = %d, want 2: %+v", len(got), got)
		}
		check := func(c cycle, start, end time.Time, dur time.Duration) {
			if !c.start.Equal(start) || c.end == nil || !c.end.Equal(end) || c.dur == nil || *c.dur != dur {
				t.Errorf("cycle = %+v, want %v..%v (%v)", c, start, end, dur)
			}
		}
		check(got[0], at(d, 10, 0, 0), at(d, 10, 4, 0), 4*time.Minute)
		check(got[1], at(d, 10, 20, 0), at(d, 11, 0, 0), 40*time.Minute)
	})

	t.Run("open cycle at range end has NULL end", func(t *testing.T) {
		got := cycles(t, at(d, 9, 0, 0), at(d, 10, 50, 0))
		if len(got) != 2 {
			t.Fatalf("cycles = %d, want 2", len(got))
		}
		if got[1].end != nil || got[1].dur != nil {
			t.Errorf("open cycle = %+v, want NULL end and duration", got[1])
		}
	})

	t.Run("cycle running at range start keeps its real start", func(t *testing.T) {
		got := cycles(t, at(d, 10, 2, 0), at(d, 10, 10, 0))
		if len(got) != 1 || !got[0].start.Equal(at(d, 10, 0, 0)) || got[0].dur == nil || *got[0].dur != 4*time.Minute {
			t.Errorf("cycles = %+v, want one starting 10:00:00 lasting 4m", got)
		}
	})

	t.Run("cycle ended before range start is not returned", func(t *testing.T) {
		got := cycles(t, at(d, 10, 10, 0), at(d, 10, 30, 0))
		if len(got) != 1 || !got[0].start.Equal(at(d, 10, 20, 0)) {
			t.Errorf("cycles = %+v, want only the 10:20 cycle", got)
		}
	})

	t.Run("look-back is bounded to 24h", func(t *testing.T) {
		// A cycle on since long before p_from starts at the first sample inside the look-back window.
		const d2 = 25
		var rows []row
		for ts := at(d2, 0, 0, 0); ts.Before(at(d2, 6, 0, 0)); ts = ts.Add(time.Minute) {
			rows = append(rows, row{ts, "heat_pump", "compressor_on", 1})
		}
		rows = append(rows, row{at(d2, 6, 0, 0), "heat_pump", "compressor_on", 0})
		insert(t, rows)
		got := cycles(t, at(d2+2, 0, 0, 0), at(d2+2, 1, 0, 0)) // 24h look-back reaches day d2+1 only
		if len(got) != 0 {
			t.Errorf("cycles = %+v, want none (cycle ended before the look-back window)", got)
		}
		got = cycles(t, at(d2, 3, 0, 0), at(d2, 7, 0, 0))
		if len(got) != 1 || !got[0].start.Equal(at(d2, 0, 0, 0)) || got[0].dur == nil || *got[0].dur != 6*time.Hour {
			t.Errorf("cycles = %+v, want one 00:00..06:00", got)
		}
	})

	t.Run("no samples", func(t *testing.T) {
		if got := cycles(t, at(d, 20, 0, 0), at(d, 21, 0, 0)); len(got) != 0 {
			t.Errorf("cycles = %+v, want none", got)
		}
	})
}

// pgxInterval scans an interval column; Valid is false for NULL.
type pgxInterval struct {
	Microseconds int64
	Valid        bool
}

func (i *pgxInterval) ScanInterval(v pgtype.Interval) error {
	i.Microseconds, i.Valid = v.Microseconds+int64(v.Days)*24*3600*1e6, v.Valid
	return nil
}
