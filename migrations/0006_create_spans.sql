-- spans: the per-EXEMPLAR-TRACE span tree (SPEC §6, S5). NOT aggregated per-name
-- across the run — each row belongs to one captured exemplar trace.
CREATE TABLE spans (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    exemplar_id    bigint  NOT NULL REFERENCES exemplars(id),
    span_id        text    NOT NULL,
    parent_span_id text,
    name           text    NOT NULL,
    self_ms        numeric NOT NULL,
    total_ms       numeric NOT NULL
);

CREATE INDEX spans_exemplar_id_idx ON spans (exemplar_id);
