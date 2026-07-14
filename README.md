# Plumber

A self-hosted performance-**audit** harness. It turns *"I think it's slow"* into a
defensible statement: **here is the p99 under this load, here is where the time
goes, and here is proof my change moved it — beyond noise, and not by accident.**

Its unit of work is the **comparison** (baseline run vs candidate run), not a live
stream. It is not an APM or a monitoring dashboard — it runs *audits*. Its
non-NIH core is three things off-the-shelf tools don't give you together: the
**evidence store**, the **verdict engine**, and the shareable **audit report**.

First audit target: **Sluice**'s OHLC/VWAP pipeline (SPEC §2).

## Status

**Pre-code.** Document suite (SPEC v3 / PLAN / CLAUDE) complete; scaffold and
toolchain in place. Next: **S0** — walking skeleton + the arch-test + migrations.
See [PROGRESS.md](PROGRESS.md) for the live state.

## Stack

- **Go 1.22+**, hexagonal layout (`internal/`), `cobra` CLI.
- **Postgres** via `jackc/pgx/v5` (pgxpool); numbered SQL migrations.
- **HDR histograms** via `HdrHistogram/hdrhistogram-go`, per-rep, persisted as
  compressed `bytea`.
- Stats: bootstrap CI on the delta (cluster resampling across reps), hand-rolled
  in Go. Explicitly **not** σ-bands (invalid on tail percentiles).
- Tests: stdlib `testing` + `testify/require`, table-driven; integration via
  `testcontainers-go` (Docker). Everything runs with `-race`; the load driver
  adds `go.uber.org/goleak`.

## Layout (SPEC §5 / CLAUDE §1)

```
cmd/plumber/        composition root — wiring + cobra commands
internal/
  domain/           pure core types (Target, SLO, Run, Result, Baseline …)
  verdict/          pure significance math (Judge, Compare)
  ports/            interfaces the app needs (consumer-defined)
  app/              orchestration: run → measure → store → judge/compare → report
  adapters/         loaddriver · trace · plan · store · report
  platform/         config · secrets · migrations runner · pg connection
migrations/         numbered SQL (append-only on evidence tables)
```

**The dependency rule:** `domain` and `verdict` import no adapter, no `pgx`, no
framework — enforced by `depguard` (`make lint`) and the arch-test (`make arch`),
not by discipline.

## Quickstart

```bash
make tools     # install golangci-lint + goimports (one-time)
make build     # -> bin/plumber
make run       # run the CLI
make test      # unit tests, race detector on
make int       # integration tests (needs Docker)
make ci        # everything CI blocks on: fmt-check lint arch test
```

## Documents (read in this order)

| File | What it is |
|---|---|
| [SPEC.md](SPEC.md) | what & why (v3) |
| [PLAN.md](PLAN.md) | slices S0–S7, tests-first, with the MVP stop-and-use gate after S3 |
| [CLAUDE.md](CLAUDE.md) | the contract every session/PR obeys — read before writing code |
| [PROGRESS.md](PROGRESS.md) | cold-session resume point; where the build actually is |

## Skills in force

Installed under `.claude/skills/` and auto-triggered by task context:

- **`backend-audit-refactor`** — the Measure → Understand → Net → Change → Verify loop.
- **`perf-measurement-rigor`** — guardrails against confidently-wrong perf numbers;
  every cardinal sin maps to a test.
- **`go-idioms`** — Go correctness reference (goroutine lifecycle, context, errors,
  Clean Architecture the Go way).

## MVP bar (SPEC §13)

Point Plumber at Sluice → run N reps under a named profile → get `p50/p99/p99.9`
with variance + error rate → declare an SLO → PASS/FAIL → mark a baseline →
change the code → re-run → get a delta with a **95% CI and a
`significant | within-noise | confounded` judgment.** When that round-trips on a
real Sluice p99, the tool has earned its keep.

## The web dashboard

The `Plumber.zip` / `Plumber-icon.zip` handoffs (SPEC §10) are **high-fidelity
design references** for a later React + TypeScript presentation layer over the
same `app` API used by the CLI. They are **not** part of the MVP (S0–S3) and are
prototypes, not production code — don't port their `support.js` / `<x-dc>` runtime.
