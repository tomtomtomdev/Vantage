# Measuring Cleanly — capture-time integrity

The goal of capture time is a set of samples that faithfully represent what a real
client would experience. Every sin here biases the *raw samples* before any
analysis runs, so no amount of clever statistics downstream can recover the truth.
Get this right first.

## 1. Coordinated omission — the one that fools everyone

**The mechanism.** A closed-loop load generator does: send request → wait for
response → send the next. Now suppose the server stalls for 1 second. During that
second, a real user population would have issued (say) 200 more requests, and
every one of them would have experienced part of that 1s stall. But the closed
client issued *zero* — it was blocked waiting. So the very requests that would
have recorded the worst latencies were never sent. The stall is recorded once,
not 200 times. Your p99 comes out beautiful, and it is a lie.

The insidious part: this biases the result *consistently* toward optimism, so it
has low variance and reads as trustworthy. It is the single most common reason a
load test says a service is fine when it isn't. (Term and analysis: Gil Tene.)

**The fix — open-model arrival.** Schedule request dispatch on a fixed timeline
that is *independent of when responses come back*. Arrivals fire on a clock (a
rate of R/sec = one arrival every 1/R seconds), and if the workers are all busy,
the arrival *queues* rather than being skipped.

**The measurement that makes it correct.** Latency is measured from the
**intended dispatch time**, not from when the request actually went out on the
wire:

```
latency = response_received − intended_dispatch_time
```

So a request that sat in the client's own queue for 800ms because the system was
saturated correctly accrues that 800ms. This is what captures the omitted tail.
Measuring from actual-send-time silently reintroduces the omission.

**Implementation shape (Go).** A ticker (or precomputed schedule) produces
`intended_dispatch` timestamps into a channel; a bounded worker pool consumes
them; each worker stamps completion and records `completion − intended_dispatch`.
The bound on the pool models real connection limits; the schedule's independence
from the pool is what defeats coordinated omission. Do *not* gate the next
arrival on the previous completion.

**If you must use a closed-loop tool,** HdrHistogram's
`recordValueWithExpectedInterval` back-fills the omitted samples as a correction —
better than nothing, but a genuinely open-model driver is the real fix and the
reason to build rather than wrap.

## 2. Warmup / steady-state

The first samples of a run come from a different system than the one you want to
measure: JIT hasn't compiled hot paths, the connection pool is filling, caches
are cold, the CPU may be scaling frequency. Including them fattens the tail with
transient effects that no production steady state exhibits.

**Fix:** discard a warmup window and record only steady-state samples. Size it to
whatever your system needs to stabilize (watch for the latency curve flattening).
Untrimmed warmup distorts the tail as surely as coordinated omission — and in the
opposite direction, so the two can mask each other.

## 3. Errors excluded from latency; error rate is co-equal

A failed request's latency is meaningless — a request that 500s in 2ms is not
"fast." Worse: past the capacity knee, a service sheds load by returning fast
errors (timeouts, 503s, circuit-breaker rejects). If those fast failures land in
your latency histogram, a service that is *actively collapsing* reports its
*best* p99 at the exact load where it's falling over.

**Fix:** compute latency percentiles over *successful* requests only, and report
error rate as a co-equal axis alongside latency. A latency number without its
error rate is not a result. On a ramp, the knee is where errors climb — plot them
together or the ceiling is invisible.

## 4. Measurement location — you might be measuring the network

Black-box / client-side latency = server processing + network RTT between the
driver and the target + the driver's own overhead. If the driver runs on your
home box and the target is in a cloud region, a large fraction of your "latency"
is your home internet, and it will swamp the signal you care about and vary with
things that have nothing to do with the service.

**Fix:** co-locate the driver with the target (same host / same VPC / same region)
for like-for-like measurement, and *record where the driver ran* as part of the
run's metadata so two runs are only compared when measured from the same vantage
point.

## 5. Driver self-overhead

If the load generator is itself the bottleneck — CPU-pinned, GC-thrashing, too few
cores to sustain the target rate — then you are measuring the driver, not the
target, and the numbers are garbage that looks fine.

**Fix:** measure and report the driver's own overhead per run (e.g. time spent in
the client between intended-dispatch and actual-send, and client CPU headroom).
If achieved rps falls short of requested rps, or client CPU is saturated, flag the
run as driver-bound and distrust its numbers. A run where the client couldn't keep
up is not a measurement of the server.

## Capture-time checklist

- [ ] Open-model arrival; next request not gated on previous completion.
- [ ] Latency measured from intended dispatch time.
- [ ] Warmup window discarded; steady-state only.
- [ ] Latency over successful requests only; error rate recorded alongside.
- [ ] Driver co-located with target; vantage point recorded.
- [ ] Driver overhead measured; driver-bound runs flagged and distrusted.
