-- Narrow readings table (ADR-0001). Idempotent: safe to re-run on a populated DB.
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS readings (
    time   timestamptz      NOT NULL,
    device text             NOT NULL,
    metric text             NOT NULL,
    value  double precision NOT NULL
);

SELECT create_hypertable('readings', by_range('time', INTERVAL '1 day'), if_not_exists => TRUE);

CREATE UNIQUE INDEX IF NOT EXISTS readings_device_metric_time_idx
    ON readings (device, metric, time DESC);

-- Enable compression only once: changing settings with compressed chunks present fails.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'readings' AND compression_enabled
    ) THEN
        ALTER TABLE readings SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'device, metric',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;
END
$$;

SELECT add_compression_policy('readings', INTERVAL '7 days', if_not_exists => TRUE);
