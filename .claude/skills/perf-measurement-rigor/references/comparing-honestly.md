# Comparing Honestly — analysis-time integrity

Capture-time integrity (`measuring-cleanly.md`) gives you honest samples. This
file is about not lying with them afterward — turning samples into percentiles,
and turning two sets of percentiles into a defensible "it got better / it didn't."

## 1. Percentiles, and storing them so they stay exact

Averages are useless for latency: one 10-second GC pause hides inside a great mean,
and the mean is dominated by the bulk while the *tail* is the user-visible product.
Report p50/p90/p99/p99.9, tail-weighted.

**Store HDR histograms, not summary numbers.** A High Dynamic Range histogram keeps
per-value-bucket counts across a huge range at fixed relative precision, so
percentiles are computed *exactly* from the recorded distribution rather than
interpolated from pre-bucketed averages. Two reasons it matters here: (a) exactness
at the tail, and (b) **mergeability** — HDR histograms add, so N repetitions
combine into a pooled distribution losslessly, and you can resample from them for
the bootstrap below. Store the compressed blob; keep per-repetition histograms, not
just pooled, because you need the between-rep variation.

## 2. Why tail percentiles are statistically unstable

The sampling error of an estimated quantile scales with `1 / f(q)` — the inverse of
the probability density at that quantile. In the tail the density is low, so the
estimate is *high variance*. Concretely, the number of samples beyond a quantile is
what pins it down:

- 60s @ 200 rps ≈ 12,000 requests.
- Beyond p99: ~120 samples. Beyond p99.9: ~12. Beyond p99.99: ~1.

A p99.9 defined by ~12 points wobbles run to run no matter how clean your capture
was. This is not a bug to fix by trying harder — it's a property of the tail. The
correct responses are: (a) size the run so the tail has enough samples when you need
a stable deep-tail number (thousands beyond the quantile → often millions of
requests), and (b) let the confidence interval below make the instability *visible*
rather than hiding it behind a confident single number.

## 3. Do not use σ-bands or t-tests on a tail percentile

Two things break normal-theory statistics exactly here:

- The sampling distribution of a tail quantile is **skewed and heavy-tailed**, not
  Gaussian. "mean ± 2σ ≈ 95%" assumes symmetry and normality the tail doesn't have.
- Comparing a candidate's single p99 to "baseline mean ± 2σ" compares a *point* to
  an *interval* and ignores the candidate's own variance entirely.

So "p99 dropped and it's outside 2 standard deviations, therefore significant" is
invalid reasoning applied to the metric you care about most. Don't.

## 4. The bootstrap CI on the delta — and the resampling unit that people get wrong

**The method.** To ask "did p99 really change?", build the sampling distribution of
the *difference* by resampling, then read a confidence interval off it:

1. Resample to produce a bootstrap replicate of the baseline's p99 and of the
   candidate's p99.
2. Record `Δ* = candidate_p99* − baseline_p99*`.
3. Repeat a few thousand times → a distribution of Δ*.
4. 95% CI = the 2.5th and 97.5th percentiles of that distribution (percentile
   method; BCa is a refinement worth adding later).
5. **Significant iff the CI excludes 0.** Otherwise report `within-noise` — the
   honest "you didn't actually move it." Report the point estimate *with* the CI,
   never alone.

**The subtle, important part — resample at the repetition level, not the request
level.** Requests *within* one run share that run's conditions (same warm cache,
same neighbour noise, same dataset), so they are correlated, not independent. If you
bootstrap by resampling individual requests from a single run, you treat that run as
the whole truth and **badly underestimate run-to-run variance** — producing a
narrow, confident CI that will be contradicted the next time you run it. Instead use
a **cluster / hierarchical bootstrap**: resample across the N repetitions (and,
optionally, resample requests within each chosen rep). This is why the harness
stores per-rep histograms and why **N=1 can never support a significance claim** —
with one rep there is no between-rep variation to observe, so any CI is fiction.
Practical floor: N ≥ 5 reps for a claim; more for deep-tail metrics.

## 5. Environment comparability — precision is not causation

A tight CI proves your *measurement* is precise. It says nothing about *why* the
number differs. If the baseline ran Tuesday and the candidate Friday, and in between
the dataset grew, the cache warmed, a neighbour VM got noisy, or a nightly job
fired, then you have a precise measurement of a *confounded* difference — the delta
is real but its cause is not your change.

This is acute for anything data-dependent. Database query latency is a function of
row count, and staging tables grow; a p99 taken at 1M rows and re-measured at 3M
rows is data drift wearing a regression's clothing, and the bootstrap will stamp it
`significant` with total confidence.

**Defenses:**
- Capture an **environment fingerprint** per run: relevant table row counts, schema
  version, cache state, host, and driver vantage point.
- **Compare only fingerprint-matched runs.** Material drift downgrades the verdict
  to `confounded` and names the drift, rather than reporting a clean delta.
- **Prefer back-to-back, same-host baseline/candidate runs** as the default
  protocol — the strongest practical control for ambient confounders. Interleave
  (A,B,A,B) if you're worried about slow drift during the session.

## 6. Comparability guards (encode these; don't rely on discipline)

`Compare(baseline, candidate)` should *refuse or downgrade* rather than emit a
misleading delta when:

- **Different load profile or target shape** → `incomparable`. (Comparing constant-
  100rps to a ramp is meaningless.)
- **N=1 on either side** → `incomparable`. (No observable variance → no significance.)
- **Material environment-fingerprint drift** → `confounded` (+ name the drift).
- Otherwise → bootstrap CI → `significant` (CI excludes 0) or `within-noise`.

## Analysis-time checklist

- [ ] Percentiles (tail-weighted), not means; exact via stored HDR histograms.
- [ ] Deep-tail numbers backed by enough tail samples, or their instability shown in the CI.
- [ ] No σ-bands / t-tests on tail percentiles.
- [ ] Bootstrap CI on the delta; significant only if it excludes 0.
- [ ] Cluster/hierarchical resampling across repetitions; N ≥ 5; never N=1 for a claim.
- [ ] Environment fingerprint matched; back-to-back runs; confounded comparisons flagged.
- [ ] Comparability guards enforced in code, not left to good intentions.
