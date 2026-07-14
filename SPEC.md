# Plumber — SPEC.md (v3)

> Working name: **Plumber** (a plumb line establishes true vertical — ground truth; "plumbing the depths" — investigation). Alternatives: *Gauge*, *Sextant*, *Tare*.

**Status:** Draft spec v3 (second review pass). Governs the forthcoming PLAN / CLAUDE / PROGRESS docs.
**One-liner:** A self-hosted audit harness that turns "I think it's slow" into "here's the number, here's where it lives, and here's the proof I moved it — beyond noise, and not by accident."

**v3 changelog (external-validity + strategy pass):**
- §8/§6/§7 add **environment comparability**: a bootstrap CI proves *precision*, not *causation* — a precise measurement of a confounded number (data drift, warm cache, noisy neighbour) is still wrong. Runs now carry an environment fingerprint and `Compare` guards on it.
- §7/§8/§11 add **errors-excluded-from-latency**: percentiles are computed over *successful* requests only, with error rate as a co-equal axis — otherwise a service that fails-fast past the knee looks *faster*.
- §2/§11/§13 make the **Sluice consolidation call**: Plumber's first audit target *is* Sluice, so the two backend-pivot projects reinforce instead of competing for the same evenings.
- §11/§13 name the **MVP as S1–S3** (black-box, prove-with-CI); white-box (S5/S6) is the expensive tier, deferred until the cheap tier has earned its keep.

**v2 changelog (first review — internal soundness):**
- §2 identity call (own services primary); §3/§6 Runs = N repetitions; §7 bootstrap CI replaces ±2σ; §5/§6 white-box rebuilt around tail exemplars + trace-context correlation; §8 rigor expanded; §11 re-scoped S5/S7 + CLI-first slicing.

---

## 1. Thesis

The `backend-audit-refactor` playbook has one cardinal rule — *number before narrative* — and every phase depends on an instrument that (a) produces the number, (b) stores it as tamper-evident evidence tied to a code state, and (c) renders a PASS/FAIL verdict against a declared target plus a before/after delta *that clears the noise band*. That instrument is Plumber.

Plumber is not a monitoring dashboard. Monitoring answers "how is it doing right now." Plumber answers a different, audit-shaped question: **"against this stated SLO, what is this endpoint's p99 under this load, where does the time go, and did my change actually move it beyond noise?"** It is the executable form of the playbook's Measure → Net → Verify loop.

## 2. Why build this, what it *is*, and what off-the-shelf can't do

Honest framing, because half of good engineering is not reinventing wheels:

- **k6 / Locust** generate load and give you a *single run's* report. They do not treat baselines as first-class objects, don't tie a run to a code state, and don't render a verdict against an SLO. Plumber *drives* a load engine (possibly k6) and owns everything downstream of the raw numbers.
- **Grafana / Prometheus** show live time-series. They are not built to say "run A vs run B, delta on p99 is −220ms, 95% CI [180, 255], significant." Plumber's unit of work is the *comparison*, not the *stream*.
- **Jaeger / Tempo** show individual traces. Plumber *ingests the tail exemplar* traces to attribute a run's *slow-path* latency to layers/spans, then folds that into the audit artifact.

So Plumber's defensible, non-NIH core is exactly three things off-the-shelf tools don't give you together: **the evidence store, the verdict engine, and the shareable audit report.** Everything else it can orchestrate.

**Identity call (resolved from review).** Plumber's primary subject is **services you own and can instrument** — Sluice, Orchard, and the like. That is where white-box attribution works, where the version-tagged evidence story is *true*, where the dogfooding lives (Plumber stores results in Postgres and drives load in Go; you will run Plumber against Plumber's own storage queries and `EXPLAIN ANALYZE` them), and where the blast-radius and authorization concerns of §9 mostly evaporate because it's your own staging box. **Black-box remains a supported mode** — point it at an endpoint, measure from the wire, lean on the "see APIs from the wire" strength — but it is a capability, not the product's centre of gravity. This choice is why the invariants in §8 can assume a controllable target as the default and treat black-box as the degraded case.

**First target = Sluice (consolidation call).** Plumber and Sluice are both Go, both backend-pivot vehicles, and both competing for the same evenings against a long queue of specced projects. Rather than run them in parallel, **Plumber's first audit target is Sluice's OHLC/VWAP pipeline.** Build Plumber, point it at Sluice, find the p99, break it, fix it, prove the delta with a CI. The two projects then reinforce into one interview-grade story — *"I built a real-time market-data pipeline and the rigorous harness that proves its latency, and here's the tail I moved"* — instead of two half-finished ones. Sluice is also an ideal first target because it's owned, instrumentable (white-box), and its latency is genuinely load- and data-dependent, so it exercises every part of the harness that matters.

## 3. Domain model (the nouns)

Pure domain types, no framework — Clean/hexagonal, per §8.

- **Target** — a service+endpoint under audit: base URL, request template (method, path, headers, body), auth strategy, `mode` (black-box | white-box), and a `mutating` flag (see §9 — a POST that creates orders is not safe to hammer). Belongs to an **allowlist**.
- **SLO** — a declared target expressed generically so it isn't latency-only: `{metric, threshold, unit, comparator, at_rps}`. Examples: `{p99, 150, ms, ≤, 200}`, `{error_rate, 0.1, %, <, 200}`, `{throughput, 200, rps, ≥, —}`. Declared *before* the run; the product enforces that ordering.
- **LoadProfile** — how traffic is shaped: `constant(rate, duration)`, `ramp(from, to, step)`, `spike`, `soak`. Each carries a **`warmup`** window whose samples are discarded (steady-state only; see §8). Ramp is the one that finds the knee.
- **Run** — one audit execution against a Target. **A Run is N `Repetition`s of the same LoadProfile** (default N configurable; N≥5 recommended for a claim). Immutable and append-only. Carries an optional **`version_marker`** — a git SHA when you own the code, else a deploy tag, image digest, or free-text note (black-box has no SHA, so this is nullable by design). Plus `label` ("before-fix", "after-index") and timestamps.
- **Repetition** — one pass of the LoadProfile within a Run. Produces a `RepResult`: an HDR histogram (post-warmup) plus its summary percentiles, achieved rps, and error tally. A Run's N repetitions are what make run-to-run variance *observable* rather than assumed.
- **Result** — the Run-level view derived from its Repetitions: the merged histogram (for pooled percentiles) **and** the per-rep distribution of each percentile (for the variance the verdict engine needs). A Run with N=1 is legal but is stamped `no-significance-possible` and cannot back a "significant" claim.
- **Baseline** — a Run designated as the reference point for a Target. Deltas compute against it.
- **Verdict** — two flavours: `Judge` (a single Run vs its SLOs → per-metric PASS/FAIL — *needs no baseline*) and `Compare` (baseline Run vs candidate Run → per-metric delta with a `significant | within-noise` judgment — *needs a baseline*).
- **AuditReport** — the rendered artifact: endpoint, verdict, the hotspot ("340ms of the p99's 380ms is one query, per the exemplar trace"), the evidence (tail exemplar + plan), and the before/after delta with its CI. The playbook's "map for the refactor" and "interview story."

## 4. Two modes (a capability split, not an identity split)

- **Black-box** — measure from the wire, no server access. The built-in driver sends the request template and measures **client-side** latency percentiles + error rate. Note the co-location caveat in §8: client-side latency includes network RTT, so *where Plumber runs* is part of the measurement. Produces **RED** (Rate, Errors, Duration).
- **White-box (primary)** — you control the target. *Additionally* correlate the run to server-side traces (§5) to break the **tail-path** request into spans, and ingest `EXPLAIN (ANALYZE, BUFFERS)` + `pg_stat_statements` deltas to attribute time to specific queries. Produces **USE** (Utilization, Saturation, Errors) + span/query attribution.

Black-box tells you *what* (p99 is 380ms). White-box tells you *where* (340ms is one N+1 in the positions query) — **and specifically where the slow ones go**, not the average, which is the whole point (see §5).

## 5. Architecture (hexagonal)

```
domain/         Target, SLO, Run, Repetition, Result, Baseline, Verdict, Report  (pure, tested first)
ports/          LoadDriver, ExemplarTraceSource, PlanSource, ResultStore, ReportRenderer, Clock
adapters/
  loaddriver/   built-in Go open-model driver (injects trace context)   (fork F1)
  trace/        fetch exemplar traces by trace-id (OTLP store / Jaeger / Tempo)  (fork F3)
  plan/         Postgres EXPLAIN + pg_stat_statements delta ingest
  store/        Postgres result/baseline store
  report/       Markdown / HTML renderer (PDF deferred)
app/            orchestration: run(N reps) → measure → store → judge/compare → report
cli/            the S0–S7 vertical seam (emits numbers); see §11 slicing note
web/            React + TS dashboard, later layer over the same app API
```

Port sketches (Go), updated so white-box correlation is real, not aspirational. The driver must **emit the trace IDs it generated** so the tail exemplars can be pulled *by ID* rather than guessed from a time window (a busy server's window is full of ambient traffic):

```go
type LoadDriver interface {
    // Injects W3C traceparent per request; returns raw latencies AND the
    // trace IDs it generated, so white-box correlation is by-ID not by-window.
    Run(ctx context.Context, t Target, p LoadProfile) (RepObservation, error)
}
type ExemplarTraceSource interface {
    // The trace(s) behind the slow tail — e.g. the requests at/above p99 —
    // fetched by the IDs the driver recorded. Not an average over the window.
    TailExemplars(ctx context.Context, ids []TraceID, atPercentile float64, k int) ([]Trace, error)
}
type PlanSource interface {
    Explain(ctx context.Context, query string) (QueryPlan, error)
    StatementsDelta(ctx context.Context, window TimeWindow) ([]StmtStat, error) // pg_stat_statements before/after
}
type ResultStore interface {
    SaveRun(ctx context.Context, r Run, reps []RepResult) (RunID, error)
    Get(ctx context.Context, id RunID) (Run, Result, error)
    SetBaseline(ctx context.Context, target TargetID, id RunID) error
    Baseline(ctx context.Context, target TargetID) (RunID, bool, error)
}
```

## 6. Data model (Postgres — and yes, you'll audit these very tables)

```
targets(id, name, base_url, request_template jsonb, auth jsonb, mode,
        mutating bool, allowlisted bool, created_at)

slos(id, target_id, metric, threshold numeric, unit, comparator, at_rps, created_at)
        -- generic: handles p99/ms, error_rate/%, throughput/rps

runs(id, target_id, version_marker text NULL, label, profile jsonb,
     n_reps int, warmup_ms int, driver_overhead_ms, started_at, finished_at,
     env_fingerprint jsonb)   -- condition-comparability: {row_counts:{table:n}, schema_version,
                              --   cache_state, host, colocation} captured at run time (see §8)

reps(id, run_id, seq, started_at, histogram bytea,      -- HDR blob, post-warmup, SUCCESS-only
     p50, p90, p99, p999, max_ms, achieved_rps, error_rate, errors jsonb)
        -- one row per repetition = the raw material for variance / bootstrap
        -- latency percentiles cover successful requests only; errors tracked as a separate axis

exemplars(id, run_id, trace_id, at_percentile, captured_latency_ms)   -- white-box, the tail traces
spans(id, exemplar_id, span_id, parent_span_id, name, self_ms, total_ms)
        -- per-EXEMPLAR-TRACE span tree, NOT aggregated per-name across the run

query_attrib(id, run_id, statement_fingerprint, calls, total_ms, mean_ms, plan jsonb)
        -- from pg_stat_statements delta over the run window

baselines(target_id PK, run_id, set_at)
```

Notes: HDR histograms are stored compressed per rep, so percentiles are exact and the bootstrap (§7) can resample straight from the blobs. `reps`, `runs`, `results` are append-only — no UPDATE path (evidence integrity). Run-level pooled percentiles are derived by merging the rep histograms on read; per-rep percentiles in `reps` supply the variance.

## 7. The verdict engine (the part that makes it *prove*)

Two pure functions, the first things written under TDD:

1. **`Judge(Run, []SLO) → []MetricVerdict`** — per-metric PASS/FAIL against the declared SLO. Needs the Run's pooled Result and the SLO; **needs no baseline**. This is what turns a graph into a decision.
2. **`Compare(baseline Run, candidate Run) → Delta`** — the load-bearing one, rebuilt from the review. It does *not* use a ±σ band (a point-vs-interval comparison that assumes normality the tail percentiles don't have). Instead:
   - **Guard first:** refuse to compare Runs with mismatched LoadProfile or Target shape, refuse if either Run is N=1, and **flag on environment drift** — if the two Runs' `env_fingerprint`s diverge materially (row counts, schema version, host, cache state), a "significant" delta may be data drift or a noisy neighbour, not your fix. Divergent conditions downgrade the verdict to `confounded` and name the drift. An incomparable pair returns `incomparable`, not a misleading delta.
   - **Bootstrap the delta:** resample from the stored HDR histograms across the N repetitions to build a sampling distribution of `Δpercentile = candidate − baseline`, and report the point estimate with a **95% confidence interval**.
   - **Verdict:** `significant` iff the CI excludes zero, else `within-noise` — the honest "you didn't actually move it." Output reads: *"p99 improved 220ms, 95% CI [180, 255] → significant."*

   The bootstrap is robust to the skew and heavy tails of percentile estimators (especially p99.9, where few samples define the quantile), which is exactly why it replaces the σ-band. Optional Python sidecar per fork F2 if you want richer interval methods; the port is stat-method-agnostic.

## 8. Non-negotiable invariants (standing + product-specific)

- Spec-driven TDD; domain + verdict logic pure and framework-free; **vertical slices, each independently shippable and green** (with the CLI-first caveat in §11).
- **The product embodies the cardinal rule, correctly scoped:** a *delta / "significant"* verdict requires a stored baseline; a *PASS/FAIL SLO* verdict requires only a declared SLO (a single audited run against a target is legitimate — it just can't claim a *change*). Number-before-narrative is enforced in code.
- **Runs are immutable and version-marked** (git SHA when owned, deploy tag/digest/note in black-box). Evidence you can edit is not evidence.
- **Measurement must not lie** — the moat, expanded:
  - **Open-model / constant-arrival** load, to avoid *coordinated omission* (a closed-loop client that waits on slow responses under-reports the tail — the exact failure that would make Plumber produce comforting, wrong numbers).
  - **Warmup discard:** the profile's warmup window is dropped so only steady-state (post-JIT, post-pool-fill, post-cache-warm) samples count. Untrimmed warmup distorts the tail as surely as coordinated omission does.
  - **Driver self-overhead** is measured and reported per run; if the client is the bottleneck, the numbers are about the client, not the target.
  - **Co-location declared:** black-box latency includes network RTT, so the run records where Plumber ran relative to the target. Same-host/same-VPC for like-for-like comparisons.
  - **Environment comparability — precision is not causation.** A bootstrap CI proves the *measurement* is tight; it says nothing about *why* the number changed. A precise measurement of a confounded number (staging data grew, cache was warm from a prior run, a neighbour VM was noisy, a cron fired) is confidently wrong. Every Run captures an `env_fingerprint`; `Compare` downgrades to `confounded` on drift (§7); and back-to-back same-box baseline/candidate runs are the default protocol. For an equities backend this is acute — **query latency is a function of row count**, and staging tables grow, so a p99 at 1M rows vs 3M rows is data drift masquerading as a regression. This ranks alongside coordinated omission, arguably above it.
  - **Errors excluded from latency; error rate is a co-equal axis.** Percentiles are computed over *successful* requests only. Past the knee a service returns fast 5xx/timeouts, and a fail-fast service will otherwise show *better* p99 at the exact load where it's collapsing. The knee curve plots latency **and** error rate together; a latency number without its error rate is not a result.
  - **N repetitions + bootstrap CI, never a single run, for a claim.** One run answers "what is it now"; a claim of change requires variance.
  - **Comparability guard** in `Compare` (see §7) — no cross-profile, N=1, or environment-drift deltas.
- **Data safety:** mutating Targets are flagged and treated per §9; a load run must never silently pollute real data.

## 9. Blast radius & data safety (a real guardrail, not boilerplate)

Plumber generates load. Pointed at the wrong host it *is* an availability attack; pointed at a mutating endpoint it is a *data* incident. At a securities firm this matters more, not less:

- **Allowlist-only.** No allowlist entry → no load, hard stop.
- **Staging by default.** Production targets require a separate deliberate flag and a low rate cap.
- **Rate ceiling + kill switch.** Every profile has a max-rps ceiling and a single-key abort that drains in-flight and stops.
- **Mutating-endpoint protection.** A Target flagged `mutating` is refused by default; running it requires an explicit override and is intended only for disposable/sandboxed data. Never hammer a POST that creates real orders.
- **Auth token lifecycle.** For soak-length runs the driver must refresh credentials mid-run (or fail loudly) rather than silently logging a run of 401s as "errors."
- **Authorized targets only.** Never load-test a system you don't own or aren't explicitly cleared to test. Spec-level rule, not a config default.

## 10. Frontend surfaces (React + TS, Recharts or similar — later layer, see §11)

- **Targets** — list, add, configure request template + SLO; allowlist & mutating status visible.
- **Run** — pick profile + N, trigger, live progress (achieved rps, running error rate, reps completed).
- **Result** — latency distribution (histogram + percentile line), the **throughput-vs-latency knee curve** (ramp), error taxonomy. White-box: the **tail-exemplar span waterfall** (the flame-graph analogue for the *slow* request) and per-query attribution table.
- **Compare** — baseline vs candidate side-by-side, delta with its **CI**, and a big `SIGNIFICANT` / `WITHIN NOISE` / `INCOMPARABLE` badge.
- **Report** — export the AuditReport. Markdown first, HTML next.

## 11. Vertical slices — PLAN preview, each mapped to a playbook phase

**Slicing decision (from review):** S0–S7 ship as a **CLI vertical seam** — each slice is end-to-end (flags → domain → store → stdout/file) and independently demoable, and the CLI *is* the slice's UI. The React dashboard (§10) is a subsequent presentation layer over the same `app` API, added once the seams are proven. This deliberately trades strict UI-per-slice verticality for a CLI-as-UI seam; calling it out so it's a choice, not a silent gap.

**MVP tier (from review): S1–S3 is the product; S5–S6 is the expensive tier.** Value lands at S3 — black-box number → judge → prove-with-CI — which is fully useful without any white-box work. Ship S1–S3, point them at Sluice, and *actually use them* before committing to the S5/S6 effort explosion (trace correlation, tail capture). Let the cheap tier earn its keep first; S4 is a natural extension, S5–S6 are opt-in depth. §13 tiers the success criteria accordingly.

- **S0 — walking skeleton.** Domain types + Postgres store + one Target config (Sluice's pipeline endpoint). No load yet. *(scaffolding)*
- **S1 — black-box constant load, N reps → per-rep success-only histograms → stored Run** *(with `env_fingerprint`)*. The minimum that produces a real number *with variance*. First real target: Sluice. *(Phase 1: Measure)* — **MVP**
- **S2 — SLO config + `Judge`.** PASS/FAIL against declared targets. *(Phase 1: SLO-first)* — **MVP**
- **S3 — baseline designation + `Compare` (bootstrap CI + comparability guard incl. env drift).** Now it *proves*. *(Phase 3: verify against baseline)* — **MVP; ship and use before S5**
- **S4 — ramp profile + knee detection.** Finds the ceiling. Plot latency **and error rate together** — the knee is where errors climb, and latency-only past that point lies (§8). **Knee detection is a real algorithm** (Kneedle / max-curvature), not a checkbox. *(Phase 1: the knee)*
- **S5 — white-box tail attribution.** Inject W3C trace context in the driver, record generated trace IDs, pull the **p99 exemplar traces by ID**, store per-trace span trees, render the exemplar waterfall. **The expensive tier** — trace-context correlation + tail capture, not "ingest spans." Defer until S1–S3 have proven their worth on Sluice. *(Phase 1: isolate the *slow* layer)*
- **S6 — query-plan attribution.** `pg_stat_statements` delta over the run window + `EXPLAIN` on the top offenders, with an N+1 heuristic (many similar spans / high loop-count in the exemplar). *(perf-killers.md + query-plan-reading.md, mechanised)*
- **S7 — report renderer.** **Markdown first** (trivial, shareable, diffable); **HTML** as a fast follow; **PDF deferred** — print-to-PDF from HTML is nearly free later and full PDF layout is a rabbit hole (YAGNI-suspect). *(the artifact the whole playbook is trying to produce)*

## 12. Open forks (decide before the noted slice)

- **F1 — Load generation: build vs wrap.** *(blocks S1)* Built-in Go open-model driver vs wrap k6. **Recommendation: build it.** Best Go-concurrency exercise in the project, and — decisively — you need control of the timing model to guarantee open-model arrival + warmup discard (§8), which wrapping k6 partly hands off. Wrap k6 only if time-to-first-number beats the learning.
- **F2 — Stack: Go+Postgres vs Python.** **Recommendation: Go+Postgres.** Dogfoods `query-plan-reading`, is the backend-pivot portfolio piece. If you want richer interval methods than you fancy writing in Go, put a thin Python sidecar behind the stats port (§7) rather than moving the app.
- **F3 — Exemplar trace source: OTLP store vs pull from Jaeger/Tempo.** *(blocks S5)* Now framed as *fetch-by-trace-ID* (§5), so pick whichever backend your services already emit to and can query by ID. Lower stakes than F1/F2.

## 13. Success criteria (the loop closes end-to-end)

**MVP success (S1–S3, black-box, on Sluice) — this is the bar that matters:** point Plumber at Sluice → run N reps under a named profile → get `p50/p99/p99.9` *with variance* and error rate → declare an SLO → get PASS/FAIL → mark a baseline → change the code → re-run → get a delta with a **95% CI and a `significant | within-noise | confounded` judgment**. When *that* round-trips, Plumber already earns its keep and the backend-pivot story exists. Ship here, use it, then decide on the rest.

**Full loop (adds S4–S7):** + knee curve (latency & errors) + white-box p99 exemplar span/query breakdown + exported Markdown report. This mechanises the *entire* Measure→Verify loop, but it is opt-in depth on top of a tool that's already useful — not the definition of done. (v2's single-tier criterion quietly rewarded over-building; this split fixes that.)

## 14. Non-goals (scope discipline)

- Not an APM / continuous monitor — Plumber runs *audits*, it doesn't watch prod forever. (It can *ingest* from your APM; it isn't one.)
- Not a profiler — it attributes to exemplar spans/queries and hands off to `pprof`/`EXPLAIN`, it doesn't replace them.
- Not multi-tenant SaaS — self-hosted, single-team, TrueNAS-friendly.
- Not a code-quality linter — that's the CI-gate half of the playbook (`raising-the-baseline.md`), a separate concern from performance evidence.

## 15. Next documents

- **PLAN.md** — S0–S7 expanded into task lists with F1/F2 resolved.
- **CLAUDE.md** — repo conventions, hexagonal boundaries, HDR-histogram library + serialization choice, TDD/commit-hygiene (refactor vs fix separation).
- **PROGRESS.md** — cold-session resumability log.

Candidate reusable skill once built: **`perf-measurement-rigor`** (coordinated omission, warmup/steady-state, HDR histograms, percentile-not-average, bootstrap-CI-vs-σ-band, environment/condition comparability, errors-excluded-from-latency, comparability guards) — the measurement-integrity knowledge that is easy to get subtly, confidently wrong.
