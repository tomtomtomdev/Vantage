-- baselines: the current baseline run per target (SPEC §6/§8). One row per
-- target (PK); the pointer MOVES when a new baseline is set — this is the one
-- evidence-adjacent table that is intentionally mutable (upsert on set), unlike
-- runs/reps which are append-only.
CREATE TABLE baselines (
    target_id bigint      PRIMARY KEY REFERENCES targets(id),
    run_id    bigint      NOT NULL REFERENCES runs(id),
    set_at    timestamptz NOT NULL DEFAULT now()
);
