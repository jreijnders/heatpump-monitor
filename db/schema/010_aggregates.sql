-- Generic narrow continuous aggregates (ADR-0001). Idempotent.
-- Changing a cagg later needs DROP + recreate (not CREATE OR REPLACE): keep these generic.
--
-- readings_1m: per minute, per (device, metric).
--   avg_value/min_value/max_value/last_value over the minute, first/last sample time, sample count.
--   last_value + last_time are what counter deltas (energy_1m) are built on.
-- readings_1h: hierarchical on readings_1m. avg_value is weighted by sample_count.
--
-- materialized_only = false (real-time aggregation): the not-yet-materialized tail is computed
-- from raw rows on read, so dashboards show data newer than the last refresh. Cost: tiny
-- (only the last ~minute / hour is aggregated live). Policies still keep history materialized.
-- Late data older than the policy start_offset is not re-aggregated automatically; use
-- CALL refresh_continuous_aggregate(...) manually for such backfills.

CREATE MATERIALIZED VIEW IF NOT EXISTS readings_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    device,
    metric,
    avg(value)         AS avg_value,
    min(value)         AS min_value,
    max(value)         AS max_value,
    last(value, time)  AS last_value,
    max(time)          AS last_time,
    count(*)           AS sample_count
FROM readings
GROUP BY 1, device, metric
WITH NO DATA;

ALTER MATERIALIZED VIEW readings_1m SET (timescaledb.materialized_only = false);

SELECT add_continuous_aggregate_policy('readings_1m',
    start_offset      => INTERVAL '2 hours',
    end_offset        => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists     => TRUE);

CREATE MATERIALIZED VIEW IF NOT EXISTS readings_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', bucket) AS bucket,
    device,
    metric,
    sum(avg_value * sample_count) / sum(sample_count) AS avg_value,
    min(min_value)                AS min_value,
    max(max_value)                AS max_value,
    last(last_value, bucket)      AS last_value,
    max(last_time)                AS last_time,
    sum(sample_count)             AS sample_count
FROM readings_1m
GROUP BY 1, device, metric
WITH NO DATA;

ALTER MATERIALIZED VIEW readings_1h SET (timescaledb.materialized_only = false);

SELECT add_continuous_aggregate_policy('readings_1h',
    start_offset      => INTERVAL '3 days',
    end_offset        => INTERVAL '1 hour',
    schedule_interval => INTERVAL '15 minutes',
    if_not_exists     => TRUE);

-- power_1m: per-minute average power (W) per device. Grafana reads this view, never the caggs.
CREATE OR REPLACE VIEW power_1m AS
SELECT bucket, device, avg_value AS power_w
FROM readings_1m
WHERE metric = 'power_w';

GRANT SELECT ON readings_1m, readings_1h, power_1m TO analytics_ro;
