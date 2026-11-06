// Command prices periodically fetches easyEnergy electricity and gas prices into energy_prices.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // distroless image has no zoneinfo

	"github.com/jackc/pgx/v5/pgxpool"

	"heatpump-monitor/ingester/internal/prices"
)

// retryAfterFailure: a failed sync (API or DNS hiccup) is retried sooner than the normal interval.
const retryAfterFailure = 5 * time.Minute

type config struct {
	databaseURL  string
	baseURL      string
	backfillDays int
	interval     time.Duration
	logLevel     slog.Level
}

func loadConfig() (config, error) {
	c := config{
		databaseURL:  os.Getenv("DATABASE_URL"),
		baseURL:      os.Getenv("PRICES_API_URL"),
		backfillDays: 60,
		interval:     time.Hour,
	}
	if c.databaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if c.baseURL == "" {
		c.baseURL = prices.DefaultBaseURL
	}
	if v := os.Getenv("PRICES_BACKFILL_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return c, fmt.Errorf("PRICES_BACKFILL_DAYS %q: want a non-negative integer", v)
		}
		c.backfillDays = n
	}
	if v := os.Getenv("PRICES_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute {
			return c, fmt.Errorf("PRICES_INTERVAL %q: want a duration >= 1m", v)
		}
		c.interval = d
	}
	if lv := os.Getenv("LOG_LEVEL"); lv != "" {
		if err := c.logLevel.UnmarshalText([]byte(lv)); err != nil {
			return c, fmt.Errorf("LOG_LEVEL %q: %w", lv, err)
		}
	}
	return c, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.logLevel})))
	if err := run(cfg); err != nil {
		slog.Error("prices failed", "err", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client := prices.Client{BaseURL: cfg.baseURL, HTTP: &http.Client{Timeout: 30 * time.Second}, Loc: loc}

	// First pass backfills; later passes refresh from two days back up to tomorrow (published ~13:00).
	back := cfg.backfillDays
	for {
		today := time.Now().In(loc)
		from := time.Date(today.Year(), today.Month(), today.Day()-back, 0, 0, 0, 0, loc)
		to := time.Date(today.Year(), today.Month(), today.Day()+2, 0, 0, 0, 0, loc)
		wait := cfg.interval
		if err := sync(ctx, client, pool, from, to); err != nil {
			wait = min(wait, retryAfterFailure)
			slog.Warn("price sync failed", "err", err, "retry_in", wait.String())
		} else {
			back = 2
		}
		select {
		case <-ctx.Done():
			slog.Info("stopped")
			return nil
		case <-time.After(wait):
		}
	}
}

// sync fetches both commodities for local days [from, to) and upserts them.
func sync(ctx context.Context, c prices.Client, pool *pgxpool.Pool, from, to time.Time) error {
	for _, commodity := range []string{prices.Electricity, prices.Gas} {
		ps, err := c.Fetch(ctx, commodity, from, to)
		if err != nil {
			return err
		}
		if err := prices.Upsert(ctx, pool, ps); err != nil {
			return err
		}
		slog.Info("prices stored", "commodity", commodity, "count", len(ps),
			"from", from.Format(time.DateOnly), "to", to.Format(time.DateOnly))
	}
	return nil
}
