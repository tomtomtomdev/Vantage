# Plumber — PROGRESS.md

**What this is:** the cold-session resume point. Read it *after* SPEC/PLAN/CLAUDE to know where the build actually is and what to do next. Update it at the end of every session — the entry that matters most is the MVP checkpoint (§ the first real Sluice p99 moved).

---

## Status at a glance

- **Phase:** pre-code. Document suite (SPEC v3 / PLAN / CLAUDE) complete; nothing built yet.
- **Next action:** S0 — repo skeleton + hexagonal dirs + the arch-test (CLAUDE §3) + migrations for all SPEC §6 tables. Nothing loads yet.
- **MVP target:** S0–S3 → move a real p99 on Sluice's OHLC/VWAP pipeline, logged below.
- **Blocked on nothing.** F1/F2 resolved; F3 not needed until S5.

## Slice tracker

| Slice | State | Notes |
|---|---|---|
| S0 walking skeleton | 🟨 in progress | arch-test ✅ (depguard + go/list guard, proven red→green); next: migrations §6, pgx ResultStore, `target add\|list` |
| S1 black-box load → number+variance | ⬜ not started | MVP · L · the open-model driver |
| S2 SLO + Judge | ⬜ not started | MVP · S |
| S3 baseline + Compare (bootstrap CI) | ⬜ not started | MVP · M · **stop-and-use gate** |
| S4 ramp + knee | ⬜ not started | M |
| S5 white-box tail attribution | ⬜ not started | expensive tier · resolve F3 first · L |
| S6 query-plan attribution | ⬜ not started | expensive tier · M |
| S7 report renderer (MD→HTML) | ⬜ not started | S→M |

Legend: ⬜ not started · 🟨 in progress · ✅ done+green.

## Decisions log

- **2026-07-14 — Forks resolved.** F1: build the Go open-model driver (owning the timing model is required for the coordinated-omission/warmup guarantees). F2: Go + Postgres; bootstrap stats hand-rolled in Go over HDR blobs, Python sidecar deferred behind the stats port. F3 (exemplar trace source) deferred to S5.
- **2026-07-14 — Identity + first target.** Primary subject = owned/instrumentable services; black-box is a mode, not the centre of gravity. **First audit target = Sluice**, so Plumber and Sluice reinforce into one backend-pivot story instead of competing for evenings.
- **2026-07-14 — MVP boundary.** S0–S3 is the product; ship and *use* it on Sluice before greenlighting the S5/S6 expensive tier.
- **2026-07-14 — Stats method.** Bootstrap CI on the delta (cluster/hierarchical resampling across reps), significant iff CI excludes 0. Explicitly not σ-bands (invalid on tail percentiles). Codified in the `perf-measurement-rigor` skill.

## Known issues / carried forward (SPEC review, not yet folded)

- [ ] **#5 secrets at rest — GATE.** `targets.auth` must be encrypted / reference-only before S1 touches any non-local target. No plaintext tokens (securities backend). Highest-priority carried item.
- [ ] #4 retention/archival for append-only `reps` (unbounded growth) — address at S6/ops.
- [ ] #3 query-attrib ambient-traffic bug — prefer exemplar's own DB-span SQL over global pg_stat_statements delta; handle in S6.
- [ ] #6 single-endpoint ≠ prod (contention only shows under realistic mix) — note as non-goal, don't over-claim.
- [ ] #7 adoption/politics — if ever used at work, enter as shared floor-raising infra pointed at own service first, not a dossier on colleagues.
- [ ] #8 build-vs-buy honesty — pitch as learning/portfolio, not "essential infra."
- [ ] #9 exit-friction — keep personal/portfolio or hand off early if it goes team-wide (relocation goal).

## MVP checkpoint (the entry that matters)

> _Awaiting S3. When the first real Sluice p99 is moved — record: the endpoint, the SLO, before/after p99 with the 95% CI and the `significant` verdict, what the fix was, and the exemplar/query if white-box. This is the interview story and the proof the tool works._
>
> _(empty)_

## Session log

### 2026-07-14 — S0 begins: arch-test (the boundary is now real)
- `internal/arch/arch_test.go` (`//go:build archtest`) loads each pure-core package's transitive import graph via `go list -deps -json` and fails on any `adapters/`, `pgx`, `cobra`, or `net/http` import. Chose the toolchain shell-out over `golang.org/x/tools/go/packages` — the proxy is offline, and the dep-free version keeps the pure core's own module graph trivially provable. Belt-and-braces alongside depguard (`.golangci.yml`), which is the fast gate.
- **Proven, not assumed:** injected `import _ "net/http"` into `domain` → test went red (caught it transitively); reverted → green. A guard never seen to fail is worthless.
- **Gotcha found & fixed:** the test's verdict comes from external state (`go list`) the build cache can't see, so a cached pass can mask a fresh violation. `make arch` now forces `-count=1`; caveat documented in the test.
- Non-tagged `internal/arch/doc.go` added so `go build ./...` doesn't choke on a tag-only package.
- Green: `go build`, `go vet`, `gofmt`, `make test` (race), `make arch`.
- **Next:** migrations for SPEC §6 tables (test-first via the runner), then pgx `ResultStore` (`SaveRun`/`Get`/`SetBaseline`/`Baseline`) against testcontainers Postgres, then `plumber target add|list`.

### 2026-07-14 — Scaffold + toolchain + skills installed
- Hexagonal skeleton created per SPEC §5 / CLAUDE §1: `cmd/plumber` (composition root, minimal stdlib walking skeleton), all `internal/*` packages with a `doc.go` each stating responsibility + the invariant it carries. `go build`/`go vet`/`gofmt` all green; `go run ./cmd/plumber` works.
- Toolchain: `go.mod` (module `plumber`, go 1.22), `Makefile` (build/run/test-race/cover/int/arch/lint/fmt/tidy/tools/ci), `.golangci.yml` with **depguard** enforcing the dependency rule (domain/verdict deny adapters/pgx/cobra/net-http — the fast gate), `.gitignore`, `migrations/README.md` (numbered SQL + append-only rules).
- README written. Three skills installed under `.claude/skills/`: `backend-audit-refactor`, `perf-measurement-rigor`, **`go-idioms`** (the §10 correctness reference, authored + installed now). CLAUDE §0 trigger table wired; §7 note reconciled.
- Design handoff acknowledged: React+TS dashboard (SPEC §10) is a **later presentation layer over the app API**, not MVP — kept as zip, not built.
- **Still no production behaviour written** (TDD law honoured). `golangci-lint` not yet on PATH (`make tools` installs it). Repo is not yet a git repo — `git init` when ready.
- **Next action unchanged:** S0 — write the **arch-test first** (it fails until the boundary is real), then migrations for SPEC §6 tables, then the pgx `ResultStore` + `plumber target add|list`.

### 2026-07-14 — Document suite complete
- SPEC iterated to v3: two review passes (internal soundness, then external-validity + strategy). Rebuilt the stats (bootstrap CI), the data model (Runs = N reps), white-box (tail exemplars + trace correlation); added environment-comparability and errors-excluded invariants; made the Sluice + MVP calls.
- PLAN written: S0–S7 tests-first with the MVP stop-and-use gate after S3.
- CLAUDE written: Clean-Arch-in-Go and TDD anchors; Go conventions seeded for later `go-idioms` extraction.
- Two skills built and packaged: `backend-audit-refactor`, `perf-measurement-rigor`.
- **Next session:** start S0. Write the arch-test first (it fails until the boundary is real), then the skeleton to make it pass.
