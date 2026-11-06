-- compressor_cycles(p_from, p_to): Compressor Cycles from RAW heat_pump/compressor_on samples.
-- A sample is "on" when value >= 0.5.
--   cycle_start = time of the first 'on' sample of a run; cycle_end = time of the first 'off'
--   sample after it (both are detection times, so duration is unbiased: with 10 s samples, on
--   from 10:00:00 to 10:03:50 and off at 10:04:00 gives exactly 4 min).
--   Returned: cycles overlapping [p_from, p_to), i.e. started before p_to and (still open or
--   ended after p_from), ordered by start. A cycle already running at p_from keeps its real start:
--   the function looks back up to 24 h before p_from (a cycle running longer than that is
--   truncated to the first sample in the look-back window). cycle_end is searched up to p_to only.
--   A cycle still on at the end of the range has cycle_end NULL and duration NULL.
CREATE OR REPLACE FUNCTION compressor_cycles(p_from timestamptz, p_to timestamptz)
RETURNS TABLE (cycle_start timestamptz, cycle_end timestamptz, duration interval)
LANGUAGE sql STABLE AS $$
    WITH samples AS (
        SELECT time,
               value >= 0.5 AS is_on,
               lag(value >= 0.5) OVER (ORDER BY time) AS was_on
        FROM readings
        WHERE device = 'heat_pump' AND metric = 'compressor_on'
          AND time >= p_from - INTERVAL '24 hours' AND time < p_to
    ), starts AS (
        SELECT time FROM samples WHERE is_on AND coalesce(NOT was_on, true)
    ), cycles AS (
        SELECT st.time AS started, ended.time AS ended
        FROM starts st
        LEFT JOIN LATERAL (
            SELECT min(s.time) AS time FROM samples s WHERE s.time > st.time AND NOT s.is_on
        ) ended ON true
    )
    SELECT started, ended, ended - started
    FROM cycles
    WHERE ended IS NULL OR ended > p_from
    ORDER BY started
$$;

GRANT EXECUTE ON FUNCTION compressor_cycles(timestamptz, timestamptz) TO analytics_ro;
