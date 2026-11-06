package store

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"heatpump-monitor/ingester/internal/reading"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func fast(o Options) Options {
	if o.FlushInterval == 0 {
		o.FlushInterval = 50 * time.Millisecond
	}
	if o.InitialBackoff == 0 {
		o.InitialBackoff = 20 * time.Millisecond
	}
	if o.MaxBackoff == 0 {
		o.MaxBackoff = 100 * time.Millisecond
	}
	return o
}

// start runs the store and returns a stop func that cancels and waits.
func start(t *testing.T, s *Store) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	var once bool
	stop = func() error {
		if once {
			return nil
		}
		once = true
		cancel()
		return <-done
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

func gen(device string, n int, base time.Time) []reading.Reading {
	rs := make([]reading.Reading, n)
	for i := range rs {
		rs[i] = reading.Reading{Time: base.Add(time.Duration(i) * time.Second), Device: device, Metric: "m", Value: float64(i)}
	}
	return rs
}

func count(t *testing.T, pool *pgxpool.Pool, device string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM readings WHERE device=$1`, device).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitCount(t *testing.T, pool *pgxpool.Pool, device string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := count(t, pool, device)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("device %s: got %d rows, want %d", device, got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func cleanup(t *testing.T, pool *pgxpool.Pool, device string) {
	t.Helper()
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM readings WHERE device=$1`, device) })
}

func dev(t *testing.T) string {
	d := fmt.Sprintf("test_%s_%d", t.Name(), time.Now().UnixNano())
	return d
}

func TestInsertsThousandReadings(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	s := New(pool, fast(Options{BatchSize: 500, FlushInterval: time.Hour}))
	start(t, s)

	s.Add(gen(d, 1000, time.Now().Add(-time.Hour).UTC())...)
	waitCount(t, pool, d, 1000)
}

func TestDuplicateReadingStoredOnce(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	s := New(pool, fast(Options{}))
	start(t, s)

	r := gen(d, 1, time.Now().UTC())
	s.Add(r...)
	waitCount(t, pool, d, 1)
	s.Add(r...)
	s.Add(r...)
	time.Sleep(300 * time.Millisecond) // let the duplicates flush
	if s.Pending() != 0 {
		t.Fatalf("pending = %d", s.Pending())
	}
	if got := count(t, pool, d); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}

func TestTimerFlushBelowBatchSize(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	s := New(pool, fast(Options{BatchSize: 500}))
	start(t, s)

	s.Add(gen(d, 3, time.Now().UTC())...)
	waitCount(t, pool, d, 3)
}

func TestRetriesWhenDBUnavailable(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	ctx := context.Background()
	// Break the write target deterministically: rename the table away.
	if _, err := pool.Exec(ctx, `ALTER TABLE readings RENAME TO readings_away`); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			if _, err := pool.Exec(ctx, `ALTER TABLE readings_away RENAME TO readings`); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer restore()

	s := New(pool, fast(Options{}))
	start(t, s)
	s.Add(gen(d, 5, time.Now().UTC())...)

	time.Sleep(400 * time.Millisecond) // several failed attempts
	if s.Pending() != 5 {
		t.Fatalf("pending = %d, want 5 retained during outage", s.Pending())
	}
	restore()
	waitCount(t, pool, d, 5)
	if s.Pending() != 0 {
		t.Fatalf("pending = %d after recovery", s.Pending())
	}
}

func TestBufferCapDropsOldest(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	s := New(pool, fast(Options{BufferCap: 10, BatchSize: 100, Logger: logger}))

	s.Add(gen(d, 15, time.Now().Add(-time.Hour).UTC())...) // values 0..14, not running yet
	if s.Pending() != 10 {
		t.Fatalf("pending = %d, want 10", s.Pending())
	}
	if !strings.Contains(logs.String(), "dropped=5") {
		t.Fatalf("drop count not logged: %q", logs.String())
	}
	start(t, s)
	waitCount(t, pool, d, 10)

	var minV float64
	if err := pool.QueryRow(context.Background(), `SELECT min(value) FROM readings WHERE device=$1`, d).Scan(&minV); err != nil {
		t.Fatal(err)
	}
	if minV != 5 {
		t.Fatalf("oldest kept value = %v, want 5 (0..4 dropped)", minV)
	}
}

func TestShutdownFlushesPending(t *testing.T) {
	pool := testPool(t)
	d := dev(t)
	cleanup(t, pool, d)
	s := New(pool, fast(Options{BatchSize: 500, FlushInterval: time.Hour}))
	stop := start(t, s)

	s.Add(gen(d, 7, time.Now().UTC())...)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if got := count(t, pool, d); got != 7 {
		t.Fatalf("rows = %d, want 7", got)
	}
}
