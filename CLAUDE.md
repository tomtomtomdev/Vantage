# Plumber — CLAUDE.md

**What this is:** the contract every coding session and PR obeys. Read it before writing code. It consolidates the conventions implied by SPEC.md v3 and PLAN.md into enforceable rules, so they don't drift at 11pm.
**Companions:** SPEC.md (what & why), PLAN.md (slices & order), PROGRESS.md (where we are). Installed skills in force (`.claude/skills/`): `backend-audit-refactor`, `perf-measurement-rigor`, `go-idioms` — see §0 for when each fires.
**Stack:** Go 1.22+, Postgres (`pgx/v5`), `cobra` CLI. Forks resolved: build the Go driver (F1), Go+Postgres (F2).

> **Note on the Go conventions below (§4, §7):** the `go-idioms` skill is now installed as the **correctness reference** (goroutine lifecycle, context/cancellation, error wrapping, Clean-Arch-in-Go). This doc stays the **project-specific source of truth** — where the two overlap, CLAUDE wins because it names Plumber's actual packages and invariants. After S0–S3, fold any patterns the build proved back into the skill (§10).

---

## 0. Skill triggers (auto-fire by task context)

Skills live in `.claude/skills/` and trigger on their `description`. Load the matching one **before** writing or reviewing the relevant code — don't reconstruct the guidance from memory.

| When the work is… | Skill |
|---|---|
| Writing/reviewing **any Go** — goroutines, channels, worker pools, context, error handling, package/interface boundaries, wiring | `go-idioms` |
| **Measuring or comparing performance** — the load driver, histograms, percentiles, "did the fix help?", any p99/CI claim | `perf-measurement-rigor` |
| **Auditing a slow service / refactoring / query plans / raising the CI bar** — the Measure→Net→Verify loop | `backend-audit-refactor` |

These compose: building the load driver (S1) fires `go-idioms` (concurrency) **and** `perf-measurement-rigor` (coordinated omission) together; a Sluice audit fires `backend-audit-refactor` **and** `perf-measurement-rigor`. When in doubt, load both.

---

## 1. Repo layout (SPEC §5)

```
cmd/plumber/        composition root — wiring + cobra commands, the ONLY place adapters meet domain
internal/
  domain/           Target, SLO, LoadProfile, Run, Repetition, Result, Baseline  (pure)
  verdict/          Judge, Compare (pure; the significance math)
  ports/            interfaces the domain/app needs: LoadDriver, ExemplarTraceSource, PlanSource, ResultStore, ReportRenderer, Clock
  app/              orchestration: run(N reps) → measure → store → judge/compare → report
  adapters/
    loaddriver/     open-model Go driver
    trace/          exemplar source (F3, later)
    plan/           EXPLAIN + pg_stat_statements
    store/          pgx ResultStore
    report/         markdown/html renderer
  platform/         cross-cutting: config, secrets, migrations runner, pg connection
migrations/         numbered SQL
```

`internal/` so nothing leaks as a public API. Everything is a package with a single clear responsibility.

## 2. The four laws (violate these and the PR is wrong regardless of what it does)

1. **The dependency rule holds** (§3). `domain` and `verdict` import no adapter, no `pgx`, no framework.
2. **Test-first** (§5). No production code without a failing test that demanded it.
3. **One concern per commit** (§6). Refactor and behaviour-change never share a commit.
4. **Measurement integrity is enforced in code, not hoped for** (§8). The `perf-measurement-rigor` cardinal sins are test cases, not guidelines.

## 3. Clean Architecture in Go  *(anchor)*

The idiom is different from Swift; the principle is the same. Dependencies point **inward** — adapters depend on the domain, never the reverse.

- **Ports are defined by the consumer, not the implementer.** The interface `ResultStore` lives in `internal/ports` (owned by the domain/app side) because the *app* declares what it needs. The `store` adapter *implements* it. This is "accept interfaces, return structs" applied architecturally: the pgx adapter returns a concrete `*PgStore` that happens to satisfy the port.
- **Domain and verdict are pure.** No I/O, no clock, no randomness reaching in except through a port (`Clock`; the bootstrap's RNG is injected/seedable for deterministic tests). If `verdict.Compare` needs the current time or a DB, the design is wrong — pass data in.
- **No DI framework.** Wiring happens by hand in `cmd/plumber` (the composition root): construct adapters, inject them into the app via constructors, done. Constructor injection everywhere; no globals, no service locator, no `init()` magic.
- **Enforce the boundary with a test, not discipline.** `internal/domain` and `internal/verdict` get an arch-test that scans their import graph and fails on any `adapters/`, `pgx`, `cobra`, or `net/http` import. Use `golangci-lint`'s `depguard` (deny those imports in those packages) as the fast gate, plus a small `go/packages` test as the belt-and-braces. This is S0 work and it's non-negotiable — the boundary that isn't tested will erode by S3.

**Why this shape here specifically:** the significance math (`verdict.Compare`) is the crown jewel and the thing most worth unit-testing exhaustively; keeping it pure means it's tested with fixtures in microseconds and never needs a database to prove a statistics bug is fixed.

## 4. Go conventions — errors, context, concurrency  *(anchor; go-idioms seed)*

**Errors**
- Wrap with `%w` (`fmt.Errorf("saving run: %w", err)`); inspect with `errors.Is/As`. Domain errors are typed sentinel values in `domain` (e.g. `ErrIncomparable`, `ErrConfounded`, `ErrTargetNotAllowlisted`).
- No `panic` in library code. `cmd/` may `log.Fatal` at the top level only.
- Errors are values — return them, don't log-and-continue. Log once, at the boundary.

**Context**
- `ctx context.Context` is the first parameter of anything doing I/O or spawning work. Never store it in a struct.
- Propagate it; honour cancellation. Every outbound call (HTTP to target, DB query) carries a timeout (`context.WithTimeout`) — this is also a `backend-audit-refactor` rule.
- The kill switch (SPEC §9) is context cancellation: aborting a run cancels the ctx, workers drain and exit.

**Concurrency** (the load driver is the main event — SPEC §5, `measuring-cleanly.md`)
- **Every goroutine has an owner and a guaranteed exit path.** No fire-and-forget. Use `errgroup.Group` or an explicit `sync.WaitGroup`; the owner waits.
- **Channels closed by the sender**, never the receiver. Ranged over by consumers. `select` on `ctx.Done()` in every blocking loop so cancellation is prompt.
- The worker pool is **bounded** (models connection limits); the arrival schedule is **independent of completion** (the coordinated-omission fix — `measuring-cleanly.md` §1). Do not gate the next arrival on the previous response.
- **Run every test with `-race`.** Add `go.uber.org/goleak` to driver tests to fail on leaked goroutines — a leak in the driver is a measurement bug waiting to happen.

**General**
- Accept interfaces, return concrete types. Keep interfaces small (1–3 methods) and defined where consumed.
- Zero-value-useful where you can. Prefer composition; no inheritance to miss anyway.
- `gofmt` + `goimports` are not opinions. Names: short in small scopes, descriptive at package boundaries; no stutter (`domain.Target`, not `domain.DomainTarget`).

## 5. Test-Driven Development  *(anchor)*

**Order:** domain type → verdict logic → app orchestration → adapter. Write the failing test in the innermost layer that owns the behaviour, watch it fail, make it pass, refactor under green.

- **Unit tests** cover `domain` and `verdict` with fixtures — fast, no I/O, run on every save. The bootstrap and comparability guards (`verdict.Compare`) get exhaustive table-driven coverage: known-shift, known-no-shift, N=1, profile-mismatch, env-drift.
- **Integration tests** cover adapters against a real Postgres via `testcontainers-go` (decided in S0; keep it consistent). Tagged/separated so the unit suite stays sub-second.
- **Golden-master** for the report renderer (S7): render a fixture Run, compare to an approved snapshot; any diff is a reviewed behaviour change. Scrub non-determinism (timestamps, RNG) first.
- **Table-driven** is the default (`tests := []struct{...}` + `t.Run(name, …)`). `testify/require` for asserts that should halt the test.

**The measurement-integrity tests are mandatory, not optional** (this is the whole point of building the tool):
- Coordinated-omission test: saturate a stub, assert latency-from-intended-dispatch includes queueing (S1).
- Warmup-discard, success-only-latency, driver-overhead tests (S1).
- Bootstrap-CI + cluster-resampling-over-reps + guard tests (S3).
Map: each `perf-measurement-rigor` cardinal sin → a failing test that reproduces it → the code that avoids it. A harness untested against a sin has that sin.

**Coverage stance:** floor on *new/changed* code (diff coverage), not a global number retrofitted onto legacy — per `raising-the-baseline.md`. Coverage is a floor, not a target; the tests above matter more than the percentage.

## 6. Commit & PR hygiene (from `backend-audit-refactor`)

- **Refactor commits** preserve behaviour; every test stays green. **Fix commits** start with a failing reproduction, then the fix. Never combined — a reviewer must know whether a diff is *supposed* to change behaviour.
- One concern per commit; small and bisectable.
- Message ties to the slice and names the evidence where relevant: `S3: Compare downgrades to confounded on env drift` / `perf: cut positions-query p99 380→160ms, 95% CI [48,71]`.
- CI blocks (not suggests): `go test -race ./...`, `golangci-lint`, `gofmt` check, diff-coverage floor, and the arch-test. Red = no merge.

## 7. Platform mechanics (the cold-session resumable bits)

- **HDR histograms:** `HdrHistogram/hdrhistogram-go`; persist via its compressed encoding into `reps.histogram bytea`. Keep per-rep histograms (needed for the cluster bootstrap), not just pooled.
- **Postgres:** `pgxpool`; pool size configured and recorded; prepared statements; jsonb via pgx native. Pool tuning is a real knob (a `postgres-performance` skill candidate later).
- **Migrations:** numbered SQL in `migrations/`, applied by a tiny runner in `platform/` (or `golang-migrate`). Append-only schema for evidence tables; no destructive migrations on `runs`/`reps`/`results`.
- **Secrets (SPEC review #5 — gate before any non-local target):** `targets.auth` is **encrypted at rest** — secretbox/age with a key from env/secrets-manager, or store a reference not the token. **No plaintext bearer tokens** on disk for a securities backend. Ever.
- **Config:** explicit struct, loaded once at the composition root, injected down. No reading env vars deep in a package.
- **Run it:** `make test` (`go test -race ./...`), `make lint`, `make int` (integration, containers), `make run`. Document any deviation here the moment it appears.

## 8. Invariants that live in code (cross-refs, so nothing is "just a doc")

- Number-before-narrative: a *delta* verdict requires a stored baseline; an *SLO PASS/FAIL* needs only an SLO (SPEC §8). Enforced in `app`, tested.
- Runs immutable & version-marked; no UPDATE path on evidence tables (SPEC §8, §6).
- Blast radius: allowlist check, rate ceiling, kill switch, mutating-refusal — all in the driver's entry path, refusing before any load (SPEC §9). Tested with a non-allowlisted target → hard stop.
- Every `perf-measurement-rigor` cardinal sin → a test (§5).

## 9. Definition of done (per PLAN)

A slice is done when: its tests-first suite is green; `go test -race ./...`, lint, gofmt, arch-test, and diff-coverage all pass; the slice's `plumber` subcommand works against Sluice (or a fixture); commits respect refactor/fix separation; and PROGRESS.md has an entry. MVP (S0–S3) is done when the SPEC §13 MVP round-trip has moved a real Sluice p99, logged in PROGRESS.

## 10. Extraction note

Once S0–S3 are green and the Go patterns in §3/§4/§7 have survived contact with real code, extract them into a `go-idioms` skill — scoped as a **correctness reference** (dependency rule, context/cancellation, goroutine-leak avoidance, error wrapping), not a how-to, so it doesn't do the thinking the F1 build is meant to teach. Fold back any corrections the build surfaced.
