-- exemplars: white-box tail traces captured at a percentile (SPEC §6, S5).
CREATE TABLE exemplars (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id              bigint  NOT NULL REFERENCES runs(id),
    trace_id            text    NOT NULL,
    at_percentile       numeric NOT NULL,
    captured_latency_ms numeric NOT NULL
);

CREATE INDEX exemplars_run_id_idx ON exemplars (run_id);
