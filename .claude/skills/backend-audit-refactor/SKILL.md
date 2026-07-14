---
name: backend-audit-refactor
description: Playbook for auditing a suspected-underperforming backend, refactoring a suspected-dirty codebase, and hardening a team's baseline so quality is enforced by the pipeline rather than hoped for from individuals. Use whenever the task involves diagnosing backend latency/throughput problems, deciding what to refactor and how, writing safety nets around legacy code, reading query plans, load-testing a service, or figuring out whether bad output is a system problem or a people problem. Trigger this even when the request is phrased as "why is this slow", "this codebase is a mess", "the previous engineers wrote garbage", or "how do I move from frontend to backend" — it applies to all of those. Go/Postgres-oriented but the workflow is stack-agnostic.
---

# Backend Audit & Refactor

## What this is

A playbook for the loop that a senior backend engineer runs continuously:

> **Measure** to find ground truth → **Understand** before judging → build a **Net** (tests + baselines) → **Change** in small verifiable steps → **Verify** against the baseline.

Auditing a slow service, refactoring rotten code, and raising a team's quality bar are the *same loop wearing three hats*. The audit produces the map; the refactor follows the map; the guardrails make both repeatable. Do them in this order and each step de-risks the next. Skip a step and you are guessing.

The cardinal rule, which governs every phase: **do not theorize before you measure.** This applies to code *and to people*. "The backend is slow" and "the previous engineers were sloppy" are both hypotheses you have not run `EXPLAIN ANALYZE` on yet. Treat them as such.

## When to use which phase

Read the phase that matches where the work actually is. Most real tasks touch two or three.

| The task sounds like… | Go to |
|---|---|
| "Why is this slow / underperforming?" | Phase 1 — Audit |
| "How do I move into backend / what do I already know?" | Phase 2 — Transition |
| "This codebase is dirty, where do I start?" | Phase 3 — Refactor |
| "The engineers here are unprofessional" / "how do we stop bad work shipping?" | Phase 4 — Baseline |

---

## Phase 1 — Audit (Measure)

**Goal:** replace "it feels slow" with a number, a baseline, and a location.

1. **Define the SLO before touching anything.** "Underperforming" relative to *what*? On a trading/real-time platform the target is tail latency, not the average — **p99 and p99.9**, because the request that blows past budget is the one that costs a fill. Write the target down. That number is what you will later prove you moved.
2. **Instrument with tracing.** OpenTelemetry → Jaeger/Tempo/Datadog, so one request decomposes into spans. A trace showing 12ms of app logic and 380ms in one DB span ends the argument before it starts.
3. **Isolate by layer:** ingress/TLS → app logic → data layer → downstream calls. The data layer is guilty the large majority of the time; look there first and hardest.
4. **Reproduce under load.** k6 or Locust to drive synthetic traffic and find the knee — the point where p99 hockey-sticks is your ceiling and your headline metric.

Two mental models to instrument against: **RED** for services (Rate, Errors, Duration) and **USE** for resources (Utilization, Saturation, Errors).

→ For the ranked list of what actually causes backend slowness, and how to confirm each one, read **`references/perf-killers.md`**.
→ For reading a Postgres query plan the way you read a flame graph, read **`references/query-plan-reading.md`**.

The audit output is not a rant. It is: *"p99 on endpoint X is 380ms against a 150ms SLO; 340ms of it is one N+1 in the positions query; here's the trace."* That artifact is the map for Phase 3 and — not incidentally — an interview story.

---

## Phase 2 — Transition (frontend → backend)

**Goal:** recognize how much "backend" you already own, and close the one gap that actually matters.

**Transfers directly — claim these:**
- **Clean Architecture / ports-and-adapters.** Worth *more* server-side than on the client; it's what separates seniors from people who bolt logic onto controllers.
- **Structured concurrency.** Swift actors + structured concurrency map cleanly onto Go goroutines + channels. You already think in isolated state and message passing — the hard part.
- **Seeing APIs from the wire.** HAR/mitmproxy reverse-engineering means you understand how APIs behave under real conditions — retries, malformed responses, lying contracts. Most backend juniors have only ever seen their API from the inside.
- **TDD.** Becomes the refactoring safety net in Phase 3.

**The genuinely new mental shift:** from *one user / one device / local state* to *thousands of concurrent requests / shared mutable state / failure is the default case*. Concretely: idempotency, retries with backoff, timeouts everywhere, circuit breakers, eventual consistency.

**The one gap to prioritize above all else: the database.** SQL deeply, indexing, transactions and isolation levels, query planning. This outranks learning any framework. Phase 1's `query-plan-reading.md` is the fastest on-ramp — the audit skill *is* the transition curriculum.

**Training artifact:** take a real streaming project (Kafka + OHLC/VWAP + Postgres is ideal — it hits messaging, time-series, and storage at once), then deliberately add an *audit chapter*: instrument it, load-test it, break it on purpose, fix the p99. Running Phase 1 on your own code is the portfolio piece.

---

## Phase 3 — Refactor (Understand → Net → Change → Verify)

**Goal:** improve a suspected-dirty codebase without setting fire to production or your credibility.

**Understand first — Chesterton's fence.** "Dirty" is sometimes load-bearing dirt: a weird branch handling a production edge case someone bled for. `git blame` and the commit history tell you *why* before you delete the why. Never remove a fence until you know why it was built.

**Prioritize by risk × change-frequency, not by ugliness.** The right targets are **hotspots** — files that are *both* complex *and* churn often. The ugliest file nobody ever touches is not worth your weekend. Approximate the CodeScene idea with `git log`:

```bash
# churn ranking: files by number of commits touching them
git log --format=format: --name-only --since="12 months ago" \
  | grep -v '^$' | sort | uniq -c | sort -rn | head -40
```
Cross the churn ranking against a complexity signal (lines, cyclomatic complexity, or just "this file scares people") and refactor where the two overlap.

**Build the net — characterization tests.** You don't yet know what the code is *supposed* to do, so pin down what it *currently* does with tests, then refactor underneath a green bar. Your TDD reflex, pointed at legacy code.

→ For the full characterization-test workflow (including how to write tests for code you don't understand yet), read **`references/characterization-tests.md`**.

**Change in small, separated steps — the one non-negotiable commit rule:**
- **Refactor commits** (behavior-preserving) and **bug-fix commits** (behavior-changing) are *always separate*. Mixing them means a reviewer cannot tell what changed on purpose.
- For a bug: **reproduce with a failing test first**, then fix, so the test becomes a permanent regression guard.
- For large-scale rot: **strangler fig**, don't big-bang rewrite. Wrap the old thing, replace it incrementally, keep shipping. A big-bang rewrite strands you three weeks in with nothing green.

**Verify against Phase 1's baseline.** The same load test that found the knee now proves you moved it. Fix a slow query → rerun the load test → show the p99 delta. The audit's instrumentation *is* the refactor's proof.

---

## Phase 4 — Baseline (make quality a property of the system)

**Goal:** stop bad work from shipping *without* turning into the new person who shows up and grades colleagues.

This phase carries the most social risk in the entire playbook. Handle it with the same discipline as the rest: **verify the diagnosis before you indict anyone.**

**Bad output has several causes; only one is "bad engineers."** Before concluding people are the problem, rule out: missing scaffolding (no tests/CI/review bar — competent people ship sloppy work in an environment that never caught it), constraint under fire (that N+1 might be a 2am incident hotfix), and Chesterton's-fence-for-people (`git blame` before you judge — the "bad" code might be the person who *did* care, working around something you can't see). The tell that separates real underperformance from the rest is **a pattern that persists after clear, fair feedback** — not a single ugly file. One bad commit is noise; the same defect class, repeated, after it's been flagged, with no engagement, is signal.

**Coming in as the new frontend auditor and concluding "the backend engineers are bad" is the maximally suspicious version of that claim** — to them and to your manager. Contempt is legible in review tone and Slack, and it torches credibility faster than any slow query. The audit only lands as *"here's what the system is missing,"* never *"here's who's bad."*

**The antidote is a system, not better people.** You don't fix bad work by wishing for good engineers; you build an environment where bad work can't ship silently. Quality becomes a property of the pipeline. Crucially, **every guardrail below is also the safety net Phase 3 already needs** — the CI gate that stops new bad work is the same gate that proves your refactor didn't regress. You are not building an anti-people apparatus; you are building what a healthy backend has anyway, and most "people problems" dissolve inside it.

→ For the guardrail stack (blocking CI, definition-of-done, perf gates, review-standards-as-shared-doc, observability that makes code accuse itself) and the escalation path for the rare case that *doesn't* dissolve, read **`references/raising-the-baseline.md`**.

---

## The through-line

Phases 1 and 3 are the same skill: the audit's tracing and load tests *are* the safety net the refactor needs to prove it didn't regress. Phase 2 is what you get for free by running Phase 1 on your own code. Phase 4 is Phase 3's net, installed permanently for the whole team. Run the loop — **Measure, Understand, Net, Change, Verify** — and the four scary questions ("why slow", "am I ready", "where do I start", "who's at fault") all resolve into one repeatable discipline.
