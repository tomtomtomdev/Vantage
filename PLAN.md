# Plumber — PLAN.md

**Governs:** the build. Companion to SPEC.md v3 (§refs point there; not restated here).
**Status:** Draft plan. Sequence is S0 → S7. **MVP = S0–S3** (SPEC §11/§13); stop-and-use gate after S3.
**How to read a slice:** each slice lists *tests first* (the failing tests that define done), then tasks, then the **green bar** (the observable that says "done"), then the **commit boundary**. No slice mixes a refactor with a behaviour change (SPEC §8; skill `characterization-tests.md`). Sizes are relative effort (S/M/L), not hours.

---

## Resolved forks

- **F1 — load generation: BUILD the Go open-model driver.** (SPEC §12) Wrapping k6 is off the table because the coordinated-omission + warmup guarantees (SPEC §8) require owning the timing model. This is also the headline Go-concurrency exercise.
- **F2 — stack: Go + Postgres.** (SPEC §12) Bootstrap stats (§7) hand-rolled in Go over the HDR blobs; `gonum` allowed if convenient. Python sidecar stays a *deferred option behind the stats port*, not built now.
- **F3 — exemplar trace source: DEFERRED.** Blocks S5 only (expensive tier). Decide when S5 is actually greenlit, against whatever Sluice emits to by then.

## Tech stack (detail → CLAUDE.md)

- Go 1.22+, hexagonal layout per SPEC §5. Module `plumber`.
- **Postgres** via `jackc/pgx/v5`. Migrations as plain numbered SQL + a tiny runner (or `golang-migrate` if preferred).
- **HDR histograms** via `HdrHistogram/hdrhistogram-go`; persist with its compressed encoding into `reps.histogram bytea`.
- **CLI** via `cobra` (the S0–S7 vertical seam; SPEC §11 slicing decision).
- **Tests:** stdlib `testing`, table-driven; `testify/require` for asserts. Integration tests hit a real Postgres via `testcontainers-go` (or a `docker compose` dev DB — pick in S0, keep it consistent).
- Domain + verdict packages have **zero infra imports** — enforced by a lint/arch test in S0.

## Cross-cutting conventions

- [ ] TDD: write the failing test in the domain/verdict layer before the adapter.
- [ ] Commit hygiene: refactor commits are behaviour-preserving and keep every test green; fix commits start with a failing reproduction. Never combined.
- [ ] Every slice ends green on `go test ./...` and ships a working `plumber` subcommand.
- [ ] `env_fingerprint` and success-only latency (SPEC §8) are honoured from S1 — not retrofitted.

## Guardrails to land BEFORE any run against a live/shared target

Carried from the SPEC review (items you didn't fold, but which gate real use):

- [ ] **Secrets at rest (review #5):** `targets.auth` must be encrypted at rest or a secrets-manager reference — no plaintext bearer tokens for a securities backend. Land before S1 touches any non-local target. *(S)*
- [ ] **Mutating-endpoint refusal (SPEC §9):** wired in S1's target config; a `mutating` target is refused without explicit override.
- [ ] **Allowlist + rate ceiling + kill switch (SPEC §9):** allowlist check in S1; rate ceiling and abort in S1's driver.
- [ ] Retention policy (review #4) and query-attrib ambient-traffic fix (review #3): note now, address at S6/ops — not MVP blockers.

---

# Slices

## S0 — Walking skeleton  *(scaffolding; S)*

**Goal:** empty-but-wired hexagon — domain types compile, Postgres store round-trips one Target, arch boundaries enforced. No load.

Tests first:
- [ ] Domain constructors for `Target`, `SLO`, `LoadProfile`, `Run` validate their invariants (e.g. SLO comparator/unit legal; N≥1; profile has a warmup window).
- [ ] Arch test: `domain` and `verdict` packages import nothing from `adapters/`.
- [ ] Store integration test: `SaveRun` then `Get` returns an equal Target/Run (Postgres up via chosen harness).

Tasks:
- [ ] Repo + module + hexagonal dirs (SPEC §5); `cobra` root command.
- [ ] Domain types incl. generic `SLO{metric,threshold,unit,comparator,at_rps}` (SPEC §3) and `env_fingerprint` field on `Run`.
- [ ] Migrations for all SPEC §6 tables (create them now, fill them later).
- [ ] `pgx` `ResultStore` adapter: `SaveRun`/`Get`/`SetBaseline`/`Baseline`.
- [ ] `plumber target add|list` subcommand.

**Green bar:** `plumber target add --name sluice-ohlc …` persists; `plumber target list` prints it; `go test ./...` green.
**Commit boundary:** one skeleton commit; migrations in their own commit.

## S1 — Black-box load → number with variance  *(Phase 1: Measure; MVP; L)*

**Goal:** the open-model driver produces per-rep, success-only HDR histograms with `env_fingerprint`, stored as a Run. First real target: **Sluice's OHLC/VWAP endpoint.**

Tests first:
- [ ] **Open-model scheduler test:** arrivals are dispatched on a fixed schedule independent of response completion; when workers saturate, latency is measured from *intended dispatch time*, not send time (the coordinated-omission guarantee, SPEC §8). Assert with a stubbed slow target that measured latency includes queueing delay.
- [ ] **Warmup discard test:** samples inside the warmup window are excluded from the histogram.
- [ ] **Success-only test:** non-2xx responses increment error rate and are *excluded* from latency (SPEC §8).
- [ ] Driver self-overhead is recorded per run.

Tasks:
- [ ] `LoadDriver` (SPEC §5): rate scheduler (ticker → intended-dispatch timestamps) + bounded worker pool consuming an arrival channel. Constant profile only in S1.
- [ ] Latency = `completion − intended_dispatch`; record into HDR (success-only); tally errors separately.
- [ ] Warmup window discard; N repetitions → N `reps` rows.
- [ ] `env_fingerprint` capture: host, colocation note, target row-counts/schema-version (via a pluggable probe; for Sluice, a small SQL count query).
- [ ] Guardrails: allowlist check, rate ceiling, kill switch, mutating-refusal (SPEC §9).
- [ ] `plumber run --target … --profile constant:rps=…,dur=…,warmup=… --reps N`.

**Green bar:** `plumber run` against local Sluice prints `p50/p90/p99/p99.9` + error rate per rep and pooled, persists a Run with N reps and an env fingerprint. Point it at a deliberately-throttled stub and confirm the tail reflects queueing (coordinated-omission sanity check).
**Commit boundary:** driver, storage-of-reps, and CLI as separate commits; each green.

## S2 — SLO + Judge  *(Phase 1: SLO-first; MVP; S)*

**Goal:** turn a Run into PASS/FAIL against declared SLOs. No baseline needed (SPEC §7/§8).

Tests first:
- [ ] `Judge(Run,[]SLO)` returns per-metric PASS/FAIL across latency (ms), error_rate (%), throughput (rps) — proving the generic SLO model works.
- [ ] SLO must exist *before* the run is judged (ordering enforced).

Tasks:
- [ ] Pure `verdict.Judge`.
- [ ] `plumber slo set` + judge wired into `plumber run` output.

**Green bar:** `plumber run` ends with a per-metric PASS/FAIL table against the target's SLOs.
**Commit boundary:** one commit (pure fn + CLI wiring).

## S3 — Baseline + Compare (bootstrap CI + guards)  *(Phase 3: prove; MVP — stop-and-use gate; M)*

**Goal:** prove a change moved the number *beyond noise and not by accident*.

Tests first:
- [ ] **Bootstrap test:** resampling from merged rep-histograms yields a Δ-percentile distribution; 95% CI via the percentile method; `significant` iff CI excludes 0 (SPEC §7). Use fixtures with a known shift and a known no-shift.
- [ ] **Guard tests:** mismatched profile/target → `incomparable`; N=1 on either side → `incomparable`; material `env_fingerprint` drift → `confounded` (names the drift).

Tasks:
- [ ] `verdict.Compare(baseline,candidate) → Delta{point, ci_low, ci_high, verdict}`.
- [ ] `plumber baseline set <run>` and `plumber compare <candidate>`.
- [ ] Comparability + env-drift guards (SPEC §7/§8).

**Green bar:** the full MVP round-trip (SPEC §13 MVP tier) works end-to-end on Sluice: run → judge → baseline → change code → re-run → `compare` prints `"p99 −220ms, 95% CI [180,255] → significant"`, and downgrades to `confounded` when you deliberately grow the staging table between runs.

> ### ⛔ MVP CHECKPOINT — stop here and *use it*
> Plumber now earns its keep. Run it on Sluice for real, fix an actual p99, log it in PROGRESS.md. Only greenlight S4+ after the black-box tier has proven useful (SPEC §11 MVP tier). Do not roll straight into the expensive tier.

## SD — One-step dev env + run (`make dev`)  *(infra, not a feature slice; gates the stop-and-use step; S)*

**Goal:** a fresh clone (or a cold session) reaches a running Plumber against a local Postgres with **one command** — `make dev`. Today the path is manual: install Go/Docker yourself, hand-start a Postgres, export `PLUMBER_DATABASE_URL`, `make migrate`. That friction sits directly in front of the MVP checkpoint, so it lands **before** the real-Sluice round-trip.

**Scope decision:** the compose DB is for *running the CLI*; `make int` keeps testcontainers (S0 decision stands — don't fork the test harness). No secrets involved: local dev DSN only, which also keeps the review-#5 gate untouched.

Checks first (script-level, not unit tests — the smoke check *is* the spec):
- [ ] `scripts/dev-smoke.sh`: from a clean state, `make dev` exits 0 and `plumber target list` runs against the dev DB without manual env setup.
- [ ] **Idempotent:** a second `make dev` on an already-up env exits 0 quickly (no re-create, no duplicate migrations — runner already guarantees the latter).
- [ ] **Prereq failure is clean:** with Docker down, `make dev` fails with a one-line actionable message ("start Docker"), not a compose stack trace.

Tasks:
- [ ] `docker-compose.yml`: one pinned-version Postgres service with a healthcheck, non-default port to avoid clashing with a system Postgres.
- [ ] `.env.example` with the matching `PLUMBER_DATABASE_URL`; `make dev` copies to `.env` if absent and sources it.
- [ ] `make dev`: prereq check (go, docker) → `docker compose up -d --wait` → `plumber migrate` → `make build` → print next steps (`bin/plumber target add …`).
- [ ] `make dev-down` (stop, keep data) and `make dev-nuke` (stop + drop volume — destructive, named accordingly).
- [ ] README Quickstart rewritten to lead with `make dev`.

**Green bar:** on a machine with only Go + Docker, `git clone && make dev && bin/plumber target list` works end-to-end; smoke script green; re-run idempotent.
**Commit boundary:** compose + env in one commit; Make targets + smoke script in another; README in the last.

## S4 — Ramp + knee  *(Phase 1: the knee; M)*

**Goal:** find the capacity ceiling; plot latency **and** error rate.

Tests first:
- [ ] Ramp profile emits increasing rates by step; each step is its own measurement window.
- [ ] Knee detection (**real algorithm** — Kneedle or max-curvature; pick and unit-test on synthetic curves).
- [ ] Knee report includes error-rate axis; a fail-fast synthetic (low latency + rising errors) is *not* reported as "fast" (SPEC §8).

Tasks:
- [ ] `ramp` profile in the driver.
- [ ] Knee detector + `plumber run --profile ramp:…` output (latency + error curve, knee point).

**Green bar:** ramp against Sluice prints the knee (rps at which p99 hockey-sticks) with the co-plotted error rate.
**Commit boundary:** ramp profile, then knee detector.

## S5 — White-box tail attribution  *(EXPENSIVE TIER; resolve F3 first; L)*

**Goal:** for the p99 request specifically, show where time went. (SPEC §5/§11 — trace-context correlation + tail capture, not "ingest spans".)

Tests first:
- [ ] Driver injects W3C `traceparent` and records generated trace IDs.
- [ ] `TailExemplars(ids, atPercentile, k)` returns the trace(s) behind the tail — verified against a fixture where the slow trace is known.
- [ ] Per-exemplar span tree stored under `exemplars`/`spans` (per-trace, not aggregated).

Tasks:
- [ ] Resolve F3 (OTLP store vs Jaeger/Tempo pull).
- [ ] Trace-context injection in driver; capture tail trace IDs.
- [ ] `ExemplarTraceSource` adapter; store exemplar + span tree.
- [ ] `plumber run --white-box` populates exemplars; CLI prints a text span waterfall for the p99 exemplar.

**Green bar:** white-box run on Sluice yields the p99 exemplar's span breakdown ("340ms of 380ms in one span").
**Commit boundary:** injection, source adapter, storage, CLI — separate.

## S6 — Query-plan attribution  *(EXPENSIVE TIER; M)*

**Goal:** attribute the slow path to specific SQL + plans; flag N+1.

Tests first:
- [ ] `StatementsDelta(window)` captures pg_stat_statements before/after.
- [ ] N+1 heuristic fires on many-similar-spans / high loop-count in the exemplar (perf-killers.md / query-plan-reading.md).
- [ ] `EXPLAIN` output parsed and stored as `query_attrib.plan`.

Tasks:
- [ ] `PlanSource` adapter (`Explain`, `StatementsDelta`).
- [ ] Correlate top offenders (prefer the exemplar's own DB-span SQL over the global window — review #3) and `EXPLAIN` them.
- [ ] N+1 heuristic; CLI prints ranked query attribution + plan for the top offender.

**Green bar:** white-box run names the guilty query, its plan, and flags N+1 when present.

## S7 — Report renderer  *(the artifact; S→M)*

**Goal:** the shareable audit artifact.

Tests first:
- [ ] Markdown renderer emits a report with verdict, hotspot, evidence, and delta+CI — golden-master test on a fixture Run.
- [ ] Renderer refuses to emit a *delta* verdict without a baseline (SPEC §8 scoping) but emits an SLO PASS/FAIL without one.

Tasks:
- [ ] **Markdown first** (SPEC §11). `plumber report <run> [--vs <baseline>]`.
- [ ] **HTML** fast-follow (same model, HTML template).
- [ ] **PDF deferred** — print-from-HTML later if ever needed (YAGNI-suspect).

**Green bar:** `plumber report` produces a Markdown audit note you'd actually paste into a PR or a portfolio writeup.

---

## Definition of done

- **Per slice:** tests-first written and green; `go test ./...` green; the slice's `plumber` subcommand works against Sluice (or a fixture where Sluice isn't the point); commits respect refactor/fix separation.
- **MVP (S0–S3):** the SPEC §13 MVP round-trip works on Sluice and has been *used* to move a real p99 (logged in PROGRESS.md). This is the bar that matters.
- **Full (S0–S7):** SPEC §13 full loop — knee curve, white-box exemplar breakdown, exported report — as opt-in depth, greenlit only after MVP has earned it.

## Next docs

- **CLAUDE.md** — repo conventions, hexagonal boundary rules, HDR lib + serialization choice, migration runner, the arch-test, commit-hygiene rules.
- **PROGRESS.md** — start it at S0; the MVP checkpoint entry (the first real Sluice p99 you move) is the one that matters most.
