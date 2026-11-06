package analytics_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestReadOnlyRoleAccess(t *testing.T) {
	cfg, err := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.User, cfg.Password = "claude_ro", "claude_ro"
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	for _, v := range []string{"readings_1m", "readings_1h", "heat_pump_1m", "energy_1m", "cop_hourly",
		"cop_daily", "heat_calibration_daily", "heat_intervals_1m", "heating_curve_1h", "booster_1m", "data_freshness", "power_1m",
		"energy_prices", "breakeven_cop", "energy_cost_1m", "energy_cost_daily"} {
		t.Run("select "+v, func(t *testing.T) {
			rows, err := conn.Query(ctx, "SELECT * FROM "+v+" LIMIT 1")
			if err != nil {
				t.Fatal(err)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("execute compressor_cycles", func(t *testing.T) {
		rows, err := conn.Query(ctx, "SELECT * FROM compressor_cycles(now() - interval '1 hour', now())")
		if err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("insert denied", func(t *testing.T) {
		_, err := conn.Exec(ctx, "INSERT INTO readings VALUES (now(), 'x', 'y', 1)")
		if err == nil {
			t.Fatal("INSERT as claude_ro succeeded")
		}
	})
}

func TestDataFreshness(t *testing.T) {
	now := time.Now().UTC()
	insert(t, []row{{now.Add(-30 * time.Second), "freshness_dev", "x", 1}})
	var age float64
	if err := pool.QueryRow(context.Background(),
		"SELECT age_seconds FROM data_freshness WHERE device = 'freshness_dev'").Scan(&age); err != nil {
		t.Fatal(err)
	}
	if age < 25 || age > 120 {
		t.Errorf("age_seconds = %v, want ~30", age)
	}
}
