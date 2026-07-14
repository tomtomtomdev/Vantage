# migrations/

Numbered SQL, applied by a tiny runner in `internal/platform` (or `golang-migrate`
if preferred — decide in S0, keep it consistent). CLAUDE.md §7.

## Rules

- **Numbered, ordered:** `0001_create_targets.sql`, `0002_create_runs.sql`, …
- **Append-only on evidence tables.** `runs`, `reps`, `results` are immutable and
  version-marked (SPEC §8). No destructive migrations on them — no dropping
  columns, no rewriting rows. Corrections are new rows, not UPDATEs.
- **Secrets gate (SPEC review #5).** `targets.auth` is encrypted at rest or a
  reference — the schema must never invite a plaintext bearer token column.

## Tables (SPEC §6, created in S0)

targets · slos · load_profiles · runs · reps (histogram bytea) · results ·
baselines. Per-rep HDR histograms are kept (needed for the cluster bootstrap),
not just a pooled one.
