// Package store batches readings and writes them idempotently to TimescaleDB.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"heatpump-monitor/ingester/internal/reading"
)

const insertSQL = `INSERT INTO readings (time, device, metric, value)
SELECT * FROM unnest($1::timestamptz[], $2::text[], $3::text[], $4::float8[])
ON CONFLICT DO NOTHING`

// Options tunes the writer. Zero values fall back to production defaults.
type Options struct {
	BatchSize      int           // flush when this many rows are pending (default 500)
	FlushInterval  time.Duration // flush at least this often (default 2s)
	BufferCap      int           // max pending rows; oldest dropped beyond (default 100000)
	InitialBackoff time.Duration // first retry delay after a failed flush (default 500ms)
	MaxBackoff     time.Duration // retry delay cap (default 30s)
	Logger         *slog.Logger  // default slog.Default()
}

// finalFlushLimit bounds the flush attempt on shutdown.
const finalFlushLimit = 10 * time.Second

func (o Options) withDefaults() Options {
	if o.BatchSize <= 0 {
		o.BatchSize = 500
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = 2 * time.Second
	}
	if o.BufferCap <= 0 {
		o.BufferCap = 100_000
	}
	if o.InitialBackoff <= 0 {
		o.InitialBackoff = 500 * time.Millisecond
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 30 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// Store is a batching writer for readings. Safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
	opts Options

	mu   sync.Mutex
	buf  []reading.Reading
	head uint64 // sequence number of buf[0]; grows as rows leave the buffer
	wake chan struct{}
}

// New returns a Store writing through pool.
func New(pool *pgxpool.Pool, opts Options) *Store {
	return &Store{pool: pool, opts: opts.withDefaults(), wake: make(chan struct{}, 1)}
}

// Add queues readings without blocking. When the buffer is full the oldest
// rows are dropped and the count is logged.
func (s *Store) Add(rs ...reading.Reading) {
	if len(rs) == 0 {
		return
	}
	s.mu.Lock()
	s.buf = append(s.buf, rs...)
	dropped := 0
	if over := len(s.buf) - s.opts.BufferCap; over > 0 {
		dropped = over
		s.buf = append(s.buf[:0:0], s.buf[over:]...)
		s.head += uint64(over)
	}
	full := len(s.buf) >= s.opts.BatchSize
	s.mu.Unlock()

	if dropped > 0 {
		s.opts.Logger.Error("store buffer full, dropped oldest readings", "dropped", dropped, "cap", s.opts.BufferCap)
	}
	if full {
		s.kick()
	}
}

// Pending returns the number of buffered rows.
func (s *Store) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buf)
}

// Run flushes on batch-full and on the interval until ctx is cancelled, then
// makes a bounded final flush attempt and returns.
func (s *Store) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.opts.FlushInterval)
	defer ticker.Stop()
	backoff := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return s.finalFlush()
		case <-ticker.C:
		case <-s.wake:
		}
		if backoff > 0 {
			// A recent flush failed: wait before retrying.
			select {
			case <-ctx.Done():
				return s.finalFlush()
			case <-time.After(backoff):
			}
		}
		if err := s.flushAll(ctx); err != nil {
			backoff = s.nextBackoff(backoff)
			s.opts.Logger.Warn("flush failed, will retry", "err", err, "pending", s.Pending(), "retry_in", backoff)
			s.kick()
			continue
		}
		backoff = 0
	}
}

// kick schedules another attempt even when no new rows arrive.
func (s *Store) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Store) nextBackoff(cur time.Duration) time.Duration {
	if cur <= 0 {
		return s.opts.InitialBackoff
	}
	return min(cur*2, s.opts.MaxBackoff)
}

func (s *Store) finalFlush() error {
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushLimit)
	defer cancel()
	if err := s.flushAll(ctx); err != nil {
		return fmt.Errorf("final flush, %d rows pending: %w", s.Pending(), err)
	}
	return nil
}

// flushAll writes batches until the buffer is empty or a write fails.
func (s *Store) flushAll(ctx context.Context) error {
	for {
		n, err := s.flushBatch(ctx)
		if err != nil || n < s.opts.BatchSize {
			return err
		}
	}
}

// flushBatch writes up to BatchSize rows and removes them from the buffer only
// on success. It returns the number of rows written.
func (s *Store) flushBatch(ctx context.Context) (int, error) {
	s.mu.Lock()
	n := min(len(s.buf), s.opts.BatchSize)
	batch := append([]reading.Reading(nil), s.buf[:n]...)
	start := s.head
	s.mu.Unlock()
	if n == 0 {
		return 0, nil
	}

	times := make([]time.Time, n)
	devices := make([]string, n)
	metrics := make([]string, n)
	values := make([]float64, n)
	for i, r := range batch {
		times[i], devices[i], metrics[i], values[i] = r.Time, r.Device, r.Metric, r.Value
	}
	if _, err := s.pool.Exec(ctx, insertSQL, times, devices, metrics, values); err != nil {
		return 0, fmt.Errorf("insert %d readings: %w", n, err)
	}

	s.mu.Lock()
	// Rows dropped for capacity while we were writing already left the buffer.
	if done := int(start + uint64(n) - s.head); done > 0 {
		done = min(done, len(s.buf))
		s.buf = append(s.buf[:0:0], s.buf[done:]...)
		s.head += uint64(done)
	}
	s.mu.Unlock()
	return n, nil
}
