-- runs: one execution of N repetitions against a target (SPEC §6).
-- Evidence: immutable and version-marked (SPEC §8) — see 0009 for the guard.
-- env_fingerprint captures condition-comparability at run time
-- ({row_counts, schema_version, cache_state, host, colocation}) so verdict.Compare
-- can downgrade to confounded on drift.
CREATE TABLE runs (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id          bigint      NOT NULL REFERENCES targets(id),
    version_marker     text,
    label              text,
    profile            jsonb       NOT NULL,
    n_reps             int         NOT NULL,
    warmup_ms          int         NOT NULL DEFAULT 0,
    driver_overhead_ms numeric,
    started_at         timestamptz NOT NULL DEFAULT now(),
    finished_at        timestamptz,
    env_fingerprint    jsonb       NOT NULL DEFAULT '{}'::jsonb,

    CONSTRAINT runs_n_reps_positive CHECK (n_reps > 0)
);

CREATE INDEX runs_target_id_idx ON runs (target_id);
