# Plumber — PROGRESS.md

**What this is:** the cold-session resume point. Read it *after* SPEC/PLAN/CLAUDE to know where the build actually is and what to do next. Update it at the end of every session — the entry that matters most is the MVP checkpoint (§ the first real Sluice p99 moved).

---

## Status at a glance

- **Phase:** S3 implemented and green (unit + arch). `verdict.Compare` proves a change beyond noise: cluster bootstrap over reps → 95% CI on Δpercentile, `significant | within-noise`, with `incomparable` (profile/N=1) and `confounded` (env drift) guards. `plumber baseline set <run>` + `plumber compare <run>` close the MVP loop. S0–S2 still stand.
- **Next action:** the MVP is code-complete. Run `make int` against Docker once (validates S0 store + S1 `SaveRun` + S2 `SetSLO` + **S3 `GetRun`/`SetBaseline`/`Baseline`** round-trips — all compile, none executed here; Docker down), then do the **stop-and-use** step: point Plumber at real Sluice, move a p99, and log it in the MVP checkpoint below.
- **MVP target:** S0–S3 → move a real p99 on Sluice's OHLC/VWAP pipeline, logged below.
- **Blocked on nothing.** F1/F2 resolved; F3 not needed until S5.

## Slice tracker

| Slice | State | Notes |
|---|---|---|
| S0 walking skeleton | 🟨 nearly done | arch-test ✅ · `domain.Target` ✅ · migrations §6 + idempotent runner ✅ · pgx `TargetStore` ✅ · `migrate` + `target add\|list` CLI ✅ · unit+arch green. **Remaining:** run `make int` against Docker (integration tests compile but weren't executed here — Docker was down). |
| S1 black-box load → number+variance | 🟨 impl green | MVP · L · open-model driver ✅ (coordinated-omission/warmup-discard/success-only/overhead tests, `-race`+`goleak`) · `internal/hist` HDR wrapper ✅ · guards (allowlist/mutating/rate-ceiling/kill-switch) ✅ · `RunService` N-reps orchestration ✅ · pgx `SaveRun`+`GetTargetByName` ✅ · env fingerprint (`HostProbe`) ✅ · `plumber run` CLI ✅ · unit+arch green. **Remaining:** `make int` under Docker (SaveRun round-trip compiles, not executed); first real Sluice run. |
| S2 SLO + Judge | 🟨 impl green | MVP · S · `domain.SLO` generic `{metric,threshold,unit,comparator,at_rps}` + `domain.Result` pooled view ✅ · pure `verdict.Judge` (latency/error_rate/throughput; `ErrNoSLO` on empty) ✅ · `ports.SLOStore` + pgx upsert (`0010` unique `target_id,metric`) ✅ · `app.SLOService` + `RunService` judges when SLOs exist ✅ · `plumber slo set\|list` + verdict table in `run` ✅ · unit+arch green. **Remaining:** `make int` under Docker (SetSLO/SLOsForTarget round-trip compiles, not executed). |
| S3 baseline + Compare (bootstrap CI) | 🟨 impl green | MVP · M · **stop-and-use gate** · pure `verdict.Compare` ✅ (cluster bootstrap over reps, percentile-method 95% CI, significant⇔CI excludes 0) · guards: incomparable (profile/target/N=1) + confounded (host/colocation/Extra drift, named) ✅ · `ports.BaselineStore` segregated + pgx `GetRun`/`SetBaseline`/`Baseline` ✅ · `app.CompareService` (decode→CompareInput) ✅ · `plumber baseline set` + `compare` (honest report shape) ✅ · unit+arch green. **Remaining:** `make int` under Docker (GetRun/baseline round-trip compiles, not executed); the real-Sluice stop-and-use round-trip (MVP checkpoint). |
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

### 2026-07-16 — S3: baseline + `verdict.Compare` (bootstrap CI + guards) — impl green
- **Verdict (crown jewel, the whole point of the tool):** pure `verdict.Compare(baseline, candidate CompareInput, …Option) Comparison`. **Cluster bootstrap over REPS, not requests** — each iteration resamples the N reps with replacement, pools their histograms, reads the percentile; a repeated rep merges twice. This propagates *between-rep* variance into the CI, which is exactly what request-level resampling would hide (the skill's cardinal subtlety; also why N=1 can't back a claim). Percentile-method 95% CI on Δ per {p50,p90,p99,p99.9}; `significant` iff CI excludes 0, else `within-noise`. **Guards first:** profile/target mismatch or N<2 → `incomparable` (no delta emitted); `env_fingerprint` drift (host, colocation, any `Extra` key — conservative exact-match) → `confounded`, naming the drift with before→after, deltas still shown (downgrade, not refusal — precision ≠ causation). RNG seedable (`WithSeed`, fixed default → reproducible/auditable CI); `WithResamples` (default 2000). Imports only `domain` + pure `hist`, so arch-test stays green and the math is fixture-tested in µs.
- **Tests = the spec (each measurement sin → a case):** known-shift→significant (CI excludes 0), known-no-shift→within-noise (CI straddles 0), **reps-not-requests** (same wide between-rep spread both sides → wide CI, would collapse to ~[0,0] under request resampling), N=1 either side→incomparable, profile mismatch→incomparable, host/colocation/row-count drift→confounded+named, determinism (same seed → identical CI).
- **Ports/domain:** segregated `ports.BaselineStore` (`GetRun`/`SetBaseline`/`Baseline`) rather than growing `ResultStore` — keeps `RunService`'s deps and its fake unchanged (CLAUDE §4, interface segregation). `domain.ErrRunNotFound`/`ErrNoBaseline` sentinels.
- **App:** `app.CompareService` resolves candidate→target→baseline, refuses with `ErrNoBaseline` when unset (a delta needs a baseline, SPEC §8), decodes both runs' histograms into `verdict.CompareInput`, delegates to the pure verdict. Unit-tested against an in-memory fake (known-shift→significant; no-baseline/not-found errors mapped).
- **Store:** pgx `GetRun` reconstructs the Run incl. per-rep histogram blobs (what the bootstrap resamples), profile + env from jsonb, missing→`ErrRunNotFound`; `SetBaseline` upserts the one intentionally-mutable pointer (`baselines`, 0008); `Baseline` maps absence to ok=false. Integration test tagged.
- **CLI:** `plumber baseline set <run-id>` designates the reference; `plumber compare <candidate-run-id> [--seed --resamples]` prints the honest report shape — per-percentile Δ + 95% CI + verdict, `CONFOUNDED — …` banner naming the drift, `INCOMPARABLE — …` refusals. Run-id parsed before any DB connect. `cli_test` covers the new tree + arg validation.
- **Green here:** `go build`, `go vet` (incl. `-tags=integration`), `gofmt -s`, `go test -race ./...`, `make arch`. CLI smoke: `compare --help`, bad run-id all clean.
- **⚠️ Gaps (carried, same as S0–S2):** Docker down → `make int` (GetRun/SetBaseline/Baseline round-trip, jsonb/histogram decode) compiled but **not executed**. `golangci-lint` not on PATH here. Secrets-at-rest gate (review #5) still open — no non-local target touched yet.
- **Next:** MVP is code-complete (S0–S3). `make int` on a Docker host, then the **stop-and-use** round-trip on real Sluice — run → judge → baseline → fix → re-run → `compare` prints the CI verdict, and grow the staging table to confirm the `confounded` downgrade. Log it in the MVP checkpoint. Only then greenlight S4+.

### 2026-07-16 — S2: SLO + Judge (impl green)
- **Domain:** `domain.SLO` — the generic `{metric, threshold, unit, comparator, at_rps}` from SPEC §3, pure. `NewSLO` refuses malformed states: unknown metric/comparator, unit that doesn't match the metric (latency⇒ms, error_rate⇒%, throughput⇒rps), negative or >100% thresholds, negative at_rps. `MetricKind`/`Comparator` enums; comparator set matches the `0002` CHECK. `SLO.Check(Result)` returns `(actual, pass)` and is the single place error_rate is converted [0,1]→%. New `domain.Result` = the pooled metric view Judge consumes (plain numbers, so verdict stays fixture-testable in µs, no histogram/DB). Table-driven tests for construction + every comparator/axis.
- **Verdict (crown jewel grows):** pure `verdict.Judge(Result, []SLO) → Verdicts` — per-metric PASS/FAIL across all three axes. `ErrNoSLO` on an empty set enforces "no verdict without a declared SLO" (number-before-narrative, correctly scoped — SPEC §8); an empty set is *not* silently "all pass". `Verdicts.AllPass()` for the overall line. arch-test confirms verdict still imports only `domain`.
- **Ports/app:** consumer-owned `ports.SLOStore` (`SetSLO` upsert-per-metric, `SLOsForTarget`). `app.SLOService` validates in the domain then resolves target-name→id before touching the store. `RunService` gained an `SLOStore` dep: after `Summarise` it loads the target's SLOs and judges **only when ≥1 exists** (a single audited run with no SLO stays legitimate, carries no verdict). `RunSummary.Result()` is the seam from measurement view → pure math. Fakes updated; `run_test` asserts a seeded p50 SLO passes.
- **Store:** `SetSLO` is an upsert on the new `(target_id, metric)` unique constraint (`migrations/0010`) — `slo set` reconfigures, doesn't append; slos is config, not evidence (0009 untouched). `SLOsForTarget` re-validates each row through `NewSLO` so a malformed persisted SLO can't reach the verdict engine. `at_rps` NULL⇔0. Integration test written+tagged.
- **CLI:** `plumber slo set <target> --metric --threshold --unit --comparator [--at-rps]` and `slo list <target>`, wired at the root. `plumber run` now prints the per-metric verdict table (`METRIC · SLO · ACTUAL · @RPS · RESULT`) + overall PASS/FAIL, or a "no SLO declared" hint. `cli_test` covers the new command tree + `slo set` flag/arg validation (fails before any DB connect).
- **Green here:** `go build`, `go vet` (incl. `-tags=integration`), `gofmt -s`, `go test -race ./...`, `make arch`, fresh migrate-ordering test (picks up `0010`). CLI smoke: `slo set --help` clean.
- **⚠️ Gaps (carried, same as S0/S1):** Docker down → `make int` (SetSLO/SLOsForTarget round-trip, upsert semantics, `0010` constraint) compiled but **not executed**. `golangci-lint` not on PATH here. Secrets-at-rest gate (review #5) still open — no non-local target touched yet.
- **Next:** `make int` on a Docker host; then S3 — baseline + `verdict.Compare` (bootstrap CI + comparability/env-drift guards), the MVP stop-and-use gate.

### 2026-07-16 — S1: open-model load driver → Run with variance (impl green)
- **`internal/hist`** — pure HDR wrapper over `HdrHistogram/hdrhistogram-go` (µs internally, ms out; gob of `Snapshot` into `reps.histogram`). Records now, resampled by `verdict` in S3. Kept out of `domain` so the pure core takes no histogram dependency; arch-test unaffected.
- **Driver (`adapters/loaddriver`) — the crown jewel.** Fixed-schedule producer emits intended-dispatch timestamps into a jobs channel buffered to N, so **arrivals never gate on completion** (coordinated-omission fix); latency = `completion − intended_dispatch`; bounded worker pool; single collector builds a **success-only** histogram + a separate error tally; warmup classified by intended time and discarded; driver overhead = mean scheduling delay. Every goroutine has an owner + ctx exit (kill switch); sender-closes-channel. **Measurement-integrity tests are the spec:** coordinated-omission (slow stub → tail reflects queueing, achieved < requested), warmup-discard (deterministic-by-index count), success-only + errors-tallied, overhead-recorded — all under `-race` + `goleak` (`httptest` targets, no real network). Guards refuse before load: non-allowlisted → `ErrTargetNotAllowlisted`, mutating → `ErrMutatingRefused`, over ceiling → `ErrRateCeiling`.
- **Domain:** `LoadProfile` (constant) + `NewConstantProfile`/`ParseProfile` (pure, table-tested); `Run`/`RepResult`/`EnvFingerprint`; `Run.Validate` (N≥1, rep-count match); guard sentinels. N=1 legal (no-significance) but storable.
- **Ports (consumer-owned):** `LoadDriver`, `ResultStore` (SaveRun only — Get/baseline land in S3 when consumed), `EnvProbe`; `TargetStore` grew `GetTargetByName`.
- **Store:** `SaveRun` inserts run + N reps in one tx (profile/env as jsonb, histogram bytea), refusing a malformed Run before touching the DB; `GetTargetByName` maps missing → `domain.ErrTargetNotFound`. Integration tests written+tagged.
- **App:** `RunService` — lookup target → N × driver.Run (seq-stamped) → env probe → assemble+persist immutable Run; a refused/failed rep persists nothing. `Summarise` merges per-rep histograms for pooled percentiles and computes the **exact** pooled error rate from success counts + tallies (never folds errors into latency).
- **CLI:** `plumber run <target> --profile constant:rps=…,dur=…,warmup=… --reps N [--max-rps 1000] [--allow-mutating] [--workers] [--request-timeout] [--version-marker] [--label] [--colocation]`. Prints per-rep + pooled `p50/p90/p99/p99.9/max`, error rate (co-equal axis), achieved rps, and driver overhead. Wired by hand at the composition root; profile parse fails before any DB connect.
- **Green here:** `go build`, `go vet` (incl. `-tags=integration`), `gofmt -s`, `go test -race ./...`, `make arch`. CLI smoke: `run --help`, bad-profile, missing-DSN all clean.
- **⚠️ Gaps (carried, same as S0):** Docker down → `make int` (SaveRun round-trip, jsonb encoding) compiled but **not executed**. `golangci-lint` not on PATH here (CI/`make tools`). Secrets-at-rest gate (review #5) still open — S1 only ran against local/httptest, no non-local target yet; **land encryption before pointing at any real auth'd Sluice.** Request template is GET-to-base-URL for now (bodies/headers/auth when a real target needs them).
- **Next:** `make int` on a Docker host + first real Sluice `run`; then S2 (SLO + `Judge`).

### 2026-07-16 — S0: schema + store + CLI (walking skeleton runs)
- **Migrations (§6):** all eight SPEC tables as numbered SQL (`0001`–`0008`) + `0009` immutability. Two invariants moved from doc to schema (CLAUDE §8): evidence tables (`runs`/`reps`/`exemplars`/`spans`/`query_attrib`) raise on UPDATE/DELETE via trigger; `targets.auth` CHECK rejects plaintext `token`/`password`/`bearer`/`secret` keys (secrets gate, SPEC review #5) while allowing `secret_ref`/encrypted envelopes.
- **Runner** (`internal/platform/migrate.go`): embeds `migrations/*.sql`, applies in numeric order, each in its own tx, recorded in `schema_migrations`, idempotent. Ordering/validation are pure over `fs.FS` → fast unit test (no Docker); DB apply + triggers + CHECK are integration-tested.
- **Store** (`internal/adapters/store`): consumer-owned `ports.TargetStore` (`AddTarget`/`ListTargets`) implemented by concrete `*PgStore` over pgxpool. Duplicate name → `domain.ErrTargetExists` (errors.Is), not a leaked pg string. `domain.Target` gained the `ErrTargetExists` sentinel.
- **App + CLI:** `app.TargetService` (validate-in-domain then persist) unit-tested against an in-memory fake. `plumber migrate` and `plumber target add|list` wired by hand in `cmd/plumber` (cobra; SIGINT/SIGTERM → root-ctx cancel = kill switch). Config read in one place; missing DSN fails clean.
- **Green here:** `go build`, `go vet` (incl. `-tags=integration`), `gofmt -s`, `go test -race ./...`, `make arch`. CLI smoke: `--help`, missing-config, missing-flag all clean.
- **⚠️ Gap:** Docker was down in this session, so the testcontainers integration tests (`make int`) **compile but were not executed**. Run `make int` on a Docker host before calling S0 fully done. `golangci-lint` also not on PATH here (CI/`make tools` installs it).
- **Next:** S1 — open-model load driver (fires `go-idioms` + `perf-measurement-rigor`).

### 2026-07-14 — S0: domain.Target (test-first, pure)
- `internal/domain/target.go` + table-driven tests (stdlib — testify deferred, proxy offline; keeps the pure core dep-free). `NewTarget` validates name/URL/mode and refuses invalid states at construction; `Mode` enum; functional options `WithMutating`/`WithAllowlisted`.
- **Safe-by-default:** the zero value is not-mutating and not-allowlisted, so the dangerous states (SPEC §9) are opt-in and greppable at call sites.
- Uses `net/url` (pure parsing) — arch-test stays green, confirming the guard forbids `net/http` specifically, not all of `net`.
- Red→green: test failed to compile (undefined NewTarget) → implemented → `go test -race` green; build/vet/fmt/arch all green.

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
