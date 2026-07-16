-- query_attrib: pg_stat_statements delta over the run window (SPEC §6, S6).
CREATE TABLE query_attrib (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id                bigint  NOT NULL REFERENCES runs(id),
    statement_fingerprint text    NOT NULL,
    calls                 bigint  NOT NULL,
    total_ms              numeric NOT NULL,
    mean_ms               numeric NOT NULL,
    plan                  jsonb
);

CREATE INDEX query_attrib_run_id_idx ON query_attrib (run_id);
