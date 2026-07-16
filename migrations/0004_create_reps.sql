-- reps: one row per repetition — the raw material for variance / bootstrap (SPEC §6/§7).
-- histogram is the compressed HDR blob, post-warmup, SUCCESS-only. Per-rep
-- histograms are kept (the cluster bootstrap resamples across reps), not just a
-- pooled one. Latency percentiles cover successful requests only; errors are a
-- separate axis. Evidence: immutable (see 0009).
CREATE TABLE reps (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id       bigint      NOT NULL REFERENCES runs(id),
    seq          int         NOT NULL,
    started_at   timestamptz NOT NULL DEFAULT now(),
    histogram    bytea       NOT NULL,
    p50          numeric,
    p90          numeric,
    p99          numeric,
    p999         numeric,
    max_ms       numeric,
    achieved_rps numeric,
    error_rate   numeric,
    errors       jsonb       NOT NULL DEFAULT '{}'::jsonb,

    CONSTRAINT reps_run_seq_unique UNIQUE (run_id, seq)
);
