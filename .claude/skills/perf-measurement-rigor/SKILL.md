---
name: perf-measurement-rigor
description: >-
  Guardrails for producing performance numbers that are actually true — latency
  benchmarks, load tests, "is this fix faster?" comparisons, p99/p99.9 tail
  measurement, and before/after regression checks. Use this whenever the task
  involves measuring or comparing performance, building or reviewing a load
  generator or benchmark harness, reporting percentiles, or deciding whether a
  change moved a number. Trigger it even when the request sounds settled — "I
  benchmarked it and p99 dropped 5ms", "our load test says we're fine", "just
  measure the latency" — because the most dangerous perf numbers are the
  confidently-wrong ones, and this skill exists to catch them. Especially
  relevant when building measurement tooling (e.g. Plumber) where a subtle
  methodology bug gets baked into every future result. Stack-agnostic; Go asides
  where useful.
---

# Performance Measurement Rigor

## Why this skill exists

A performance number can be precise, reproducible, professionally presented, and
completely wrong. The failure mode isn't sloppiness — it's *methodology bugs that
bias the result in a consistent direction*, so the number looks stable while
lying. A benchmark that under-reports tail latency will under-report it every
single run, and its low variance will read as trustworthiness.

This skill is the checklist that separates a measurement you can stake a decision
on from one that just *feels* rigorous. It applies to any latency/throughput
measurement, and it is doubly important when *building* measurement tooling,
because a methodology bug in the harness contaminates every number it will ever
produce.

The one-sentence spine: **a tight confidence interval proves precision, not
truth — and truth requires that you measured the right thing, under comparable
conditions, and compared it honestly.**

## The cardinal sins (scan this first)

Each is a way to produce a stable, wrong number. Tell = how to catch it. These
are ranked by how often they silently corrupt real benchmarks.

1. **Coordinated omission.** A closed-loop client (send → await → send next) stops
   issuing requests exactly when the system stalls, so it never samples the bad
   latencies the stall would cause. *Tell:* the load generator waits for each
   response before sending the next; tail looks suspiciously clean under
   saturation. *Fix:* open-model arrival, measure from *intended* dispatch time.
   → `references/measuring-cleanly.md`
2. **Reporting the mean (or trusting it).** Averages hide the tail, and the tail
   is the product. *Tell:* a single "average latency" with no percentiles. *Fix:*
   percentiles, tail-focused (p99/p99.9), always.
3. **σ-band / t-test on a tail percentile.** The sampling distribution of p99/p99.9
   is skewed and heavy-tailed, so "mean ± 2σ" and normal-theory tests are invalid
   exactly where you use them most. *Tell:* "significant because it's outside 2
   standard deviations." *Fix:* bootstrap CI on the delta.
   → `references/comparing-honestly.md`
4. **Confounded comparison.** Baseline and candidate measured under different
   conditions (grown dataset, warm cache, noisy neighbour, different host). The
   delta is real; its *cause* isn't your change. *Tell:* the two runs are hours or
   days apart, or on different infra. *Fix:* environment fingerprint + back-to-back
   same-box runs. → `references/comparing-honestly.md`
5. **Warmup counted.** Cold JIT, empty connection pool, cold cache — the first
   samples are a different system. *Tell:* no warmup window discarded. *Fix:* drop
   a steady-state warmup window. → `references/measuring-cleanly.md`
6. **Errors folded into latency (or ignored).** Past the knee a service returns
   fast 5xx/timeouts; count those as "latency" and a *collapsing* service looks
   *fast*. *Tell:* latency reported without its error rate. *Fix:* percentiles over
   successful requests only; error rate as a co-equal axis.
   → `references/measuring-cleanly.md`
7. **Under-sampled tail.** p99.9 from a short run is defined by a handful of
   points and means almost nothing. *Tell:* a confident p99.9 from a 60-second run.
   *Fix:* size the run to the tail, or let the CI show the instability honestly.
   → `references/comparing-honestly.md`
8. **Measuring the client, not the server.** Client-side latency includes the
   driver's own overhead and the network RTT to the target. *Tell:* driver CPU
   pinned; target across the internet from the driver. *Fix:* measure driver
   overhead; co-locate. → `references/measuring-cleanly.md`

## How to use this skill

**When taking a measurement:** walk the capture-time sins (1, 5, 6, 8) before you
trust the run. Read `measuring-cleanly.md` if building the harness or if any tell
fires.

**When comparing two measurements** ("did the fix help?"): walk the analysis-time
sins (3, 4, 7) and never claim a change without a CI that excludes zero. Read
`comparing-honestly.md`.

**When building measurement tooling:** every sin above is a test case. Write the
failing test that reproduces the sin, then the code that avoids it — the
methodology *is* the spec. A harness that hasn't been tested against coordinated
omission almost certainly has it.

## The honest report shape

A performance claim worth acting on states, at minimum:

```
p99 improved 220ms → 160ms (−60ms), 95% CI [48, 71] → significant
  n = 5 reps × 60s @ 200rps open-model, 10s warmup discarded
  successful requests only; error rate 0.02% → 0.03% (within noise)
  baseline & candidate back-to-back, same host, dataset fingerprint matched
```

If a report can't fill those lines in, it hasn't earned the verb "improved" — the
right word is "looks like it might have." The gap between those two phrasings is
the entire point of this skill.

## References

- `references/measuring-cleanly.md` — capture-time integrity: open-model load &
  coordinated omission, warmup, errors-excluded, measurement location, driver
  overhead. Read when taking or building a measurement.
- `references/comparing-honestly.md` — analysis-time integrity: percentiles &
  HDR histograms, why tail-percentile variance is unstable, bootstrap CI vs
  σ-bands, cluster-resampling over repetitions, sample-size-in-the-tail,
  environment comparability, the comparability guards. Read when comparing runs.
