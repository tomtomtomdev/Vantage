-- A target declares at most one SLO per metric, so `slo set` is a reconfigure
-- (upsert) rather than an append (SPEC §3 — "configure ... SLO"). slos is not an
-- evidence table (0009 covers runs/reps/exemplars/spans/query_attrib), so an
-- UPDATE on re-declaration is intentional and allowed.
ALTER TABLE slos ADD CONSTRAINT slos_target_metric_unique UNIQUE (target_id, metric);
