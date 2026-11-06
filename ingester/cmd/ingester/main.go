// Command ingester subscribes to MQTT and writes decoded readings to TimescaleDB.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5/pgxpool"

	"heatpump-monitor/ingester/internal/reading"
	"heatpump-monitor/ingester/internal/store"
)

var topics = []string{"heatpump-monitor/gateway/+", "heatbooster/+/+"}

type config struct {
	mqttURL, clientID, mqttUser, mqttPass, databaseURL string
	logLevel                                           slog.Level
}

func loadConfig() (config, error) {
	c := config{
		mqttURL:     os.Getenv("MQTT_URL"),
		clientID:    os.Getenv("MQTT_CLIENT_ID"),
		mqttUser:    os.Getenv("MQTT_USERNAME"),
		mqttPass:    os.Getenv("MQTT_PASSWORD"),
		databaseURL: os.Getenv("DATABASE_URL"),
	}
	if c.mqttURL == "" || c.databaseURL == "" {
		return c, errors.New("MQTT_URL and DATABASE_URL are required")
	}
	if c.clientID == "" {
		c.clientID = "heatpump-monitor-ingester"
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
		slog.Error("ingester failed", "err", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := connectDB(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	writer := store.New(pool, store.Options{})
	storeCtx, cancelStore := context.WithCancel(context.Background())
	storeDone := make(chan error, 1)
	go func() { storeDone <- writer.Run(storeCtx) }()

	gate := &addGate{writer: writer}
	client := newMQTTClient(cfg, gate)
	client.Connect() // ConnectRetry keeps trying in the background; OnConnect logs success

	<-ctx.Done()
	slog.Info("shutting down")
	client.Disconnect(500)
	gate.close() // no handler can Add after this, so the final flush sees every row
	cancelStore()
	if err := <-storeDone; err != nil {
		return err
	}
	slog.Info("stopped", "pending", writer.Pending())
	return nil
}

// connectDB retries until the database accepts a ping or ctx is cancelled.
func connectDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	for delay := time.Second; ; delay = min(delay*2, 15*time.Second) {
		pool, err := pgxpool.New(ctx, url)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				slog.Info("database connected")
				return pool, nil
			}
			pool.Close()
		}
		slog.Warn("database not reachable, retrying", "err", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

func newMQTTClient(cfg config, gate *addGate) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.mqttURL).
		SetClientID(cfg.clientID).
		SetCleanSession(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOrderMatters(false)
	if cfg.mqttUser != "" {
		opts.SetUsername(cfg.mqttUser).SetPassword(cfg.mqttPass)
	}
	handler := gate.handler()
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		slog.Info("mqtt connected", "broker", cfg.mqttURL)
		filters := make(map[string]byte, len(topics))
		for _, t := range topics {
			filters[t] = 1
		}
		if tok := c.SubscribeMultiple(filters, handler); tok.Wait() && tok.Error() != nil {
			slog.Error("mqtt subscribe failed", "err", tok.Error())
			return
		}
		slog.Info("mqtt subscribed", "topics", topics)
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		slog.Warn("mqtt connection lost", "err", err)
	})
	return mqtt.NewClient(opts)
}

// addGate lets in-flight MQTT handlers finish and rejects any that start
// after close, so nothing calls Add once the store's final flush begins.
type addGate struct {
	writer *store.Store
	mu     sync.RWMutex
	closed bool
}

// close blocks until in-flight handlers return, then rejects new ones.
func (g *addGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

func (g *addGate) handler() mqtt.MessageHandler {
	return func(_ mqtt.Client, m mqtt.Message) {
		g.mu.RLock()
		defer g.mu.RUnlock()
		if g.closed {
			return
		}
		rs, err := reading.Decode(m.Topic(), m.Payload(), m.Retained(), time.Now())
		if err != nil {
			slog.Warn("decode failed", "topic", m.Topic(), "err", err)
			return
		}
		g.writer.Add(rs...)
	}
}
