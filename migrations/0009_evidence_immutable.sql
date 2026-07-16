-- Evidence integrity (SPEC §8, CLAUDE §8): runs/reps and the white-box evidence
-- tables are append-only. No UPDATE or DELETE path — corrections are new rows.
-- Enforce it in the database so the invariant is code, not convention: a stray
-- UPDATE raises instead of silently rewriting history.
--
-- NOT applied to: targets/slos (config, editable) or baselines (a moving pointer).
CREATE FUNCTION forbid_evidence_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'table % is append-only evidence (SPEC §8): % refused',
        TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER runs_immutable         BEFORE UPDATE OR DELETE ON runs
    FOR EACH ROW EXECUTE FUNCTION forbid_evidence_mutation();
CREATE TRIGGER reps_immutable         BEFORE UPDATE OR DELETE ON reps
    FOR EACH ROW EXECUTE FUNCTION forbid_evidence_mutation();
CREATE TRIGGER exemplars_immutable    BEFORE UPDATE OR DELETE ON exemplars
    FOR EACH ROW EXECUTE FUNCTION forbid_evidence_mutation();
CREATE TRIGGER spans_immutable        BEFORE UPDATE OR DELETE ON spans
    FOR EACH ROW EXECUTE FUNCTION forbid_evidence_mutation();
CREATE TRIGGER query_attrib_immutable BEFORE UPDATE OR DELETE ON query_attrib
    FOR EACH ROW EXECUTE FUNCTION forbid_evidence_mutation();
