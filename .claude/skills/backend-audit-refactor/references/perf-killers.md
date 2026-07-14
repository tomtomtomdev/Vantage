# Performance Killers — ranked, with confirmation steps

Ordered roughly by how often each is the actual culprit. For each: the symptom, how to *confirm* it (never assume), and the fix direction. Confirm before fixing — a fix aimed at the wrong killer is wasted work and a false "we tried that."

## Tier 1 — the data layer (guilty most of the time)

### N+1 queries
- **Symptom:** latency scales with result-set size; trace shows many near-identical small DB spans in a loop.
- **Confirm:** count queries per request (Postgres `pg_stat_statements`, or app-level query logging). One list endpoint firing 1 + N queries is the fingerprint.
- **Fix:** batch/join, eager-load, or a single `WHERE id = ANY($1)`. In Go, watch for a query inside a `range` loop over rows.

### Missing or unused indexes
- **Symptom:** a single query is slow; `EXPLAIN` shows `Seq Scan` on a large table.
- **Confirm:** `EXPLAIN ANALYZE` (see `query-plan-reading.md`). Check `pg_stat_user_indexes` for indexes that are never scanned (idx_scan = 0) — those cost writes and buy nothing.
- **Fix:** add the index the predicate needs; drop dead indexes. Composite index column order must match the query's filter+sort.

### Unbounded result sets
- **Symptom:** memory spikes, slow serialization, latency grows over time as tables grow.
- **Confirm:** grep for `SELECT` without `LIMIT` on a hot path; check response payload sizes.
- **Fix:** paginate (keyset/seek pagination over OFFSET for deep pages). Never return "all rows" on an endpoint that can grow.

### Lock contention
- **Symptom:** latency spikes under concurrency but a single request is fine; p99 ≫ p50 with no CPU/IO saturation.
- **Confirm:** Postgres `pg_locks`, `pg_stat_activity` for `wait_event_type = 'Lock'`; look for long transactions holding rows.
- **Fix:** shorten transactions, narrow lock scope, reconsider isolation level, avoid `SELECT ... FOR UPDATE` on hot rows.

### Connection-pool exhaustion
- **Symptom:** latency cliff at a specific concurrency level; requests queue waiting for a connection.
- **Confirm:** pool metrics (in-use vs max, wait count/time). If wait-for-connection time dominates the span, this is it.
- **Fix:** right-size the pool (more is not always better — it can overload the DB), fix connection leaks, add a pooler (PgBouncer) if churn is high.

## Tier 2 — service and I/O

### Synchronous fan-out with no timeouts / circuit breakers
- **Symptom:** one slow downstream drags every request; cascading failures.
- **Confirm:** trace shows serial downstream spans; check for missing `context.WithTimeout` on outbound calls.
- **Fix:** timeouts on *every* outbound call, circuit breakers, parallelize independent calls, degrade gracefully.

### No caching on a hot read
- **Symptom:** the same query runs thousands of times/sec with unchanging results.
- **Confirm:** `pg_stat_statements` top query by call count; is the input space small and the data slow-changing?
- **Fix:** cache (in-process LRU or Redis) with a sane TTL and invalidation story. Beware stampedes — use single-flight.

### Blocking I/O on a hot path
- **Symptom:** goroutines/threads pile up; throughput ceilings well below CPU capacity.
- **Confirm:** goroutine dump / profiler shows blocking on syscalls or channel waits.
- **Fix:** move blocking work off the request path (queue it), use async I/O, bound concurrency with a worker pool.

## Tier 3 — runtime

### GC pauses / allocation pressure
- **Symptom:** periodic latency spikes correlated with GC cycles; high allocation rate.
- **Confirm:** Go: `pprof` heap + `GODEBUG=gctrace=1`; look for allocations in hot loops.
- **Fix:** reduce allocations (reuse buffers, `sync.Pool`, avoid unnecessary interface boxing), pre-size slices/maps.

### Serialization overhead
- **Symptom:** significant CPU in JSON marshal/unmarshal on large payloads.
- **Confirm:** CPU profile shows encoding/json dominating.
- **Fix:** smaller payloads (don't over-fetch), streaming encoders, or a faster codec where it's justified.

## Confirmation discipline

For every suspected killer, the sequence is the same: **reproduce → measure the specific thing → confirm the mechanism → fix → re-measure against the Phase 1 baseline.** If you can't reproduce it, you can't confirm you fixed it. The load test that found the knee is also the acceptance test for the fix.
