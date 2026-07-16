-- slos: a threshold a target must meet. Generic across metrics — p99/ms,
-- error_rate/%, throughput/rps — via (metric, threshold, unit, comparator). SPEC §6/§8.
CREATE TABLE slos (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id  bigint      NOT NULL REFERENCES targets(id),
    metric     text        NOT NULL,
    threshold  numeric     NOT NULL,
    unit       text        NOT NULL,
    comparator text        NOT NULL,
    at_rps     numeric,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT slos_comparator_valid CHECK (comparator IN ('<', '<=', '>', '>=', '=='))
);

CREATE INDEX slos_target_id_idx ON slos (target_id);
