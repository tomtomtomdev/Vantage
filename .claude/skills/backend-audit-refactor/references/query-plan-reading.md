# Reading a Postgres Query Plan

The `EXPLAIN ANALYZE` output is your Time Profiler for the database. This guide is the frontend→backend on-ramp: learn to read a plan the way you already read a flame graph.

## The one command

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) <your query>;
```
- `ANALYZE` actually **runs** the query and reports real timings (not just estimates). It executes the statement — never run it on a mutating query in prod without a transaction you roll back.
- `BUFFERS` shows how much data came from cache vs disk — the single most useful add-on.

Read the plan **inside-out and bottom-up**: the most-indented nodes execute first, feeding their parents.

## What each line tells you

```
Seq Scan on positions  (cost=0.00..18334.00 rows=1 width=64)
                       (actual time=0.015..250.3 rows=1 loops=1)
```
- **cost=start..total** — planner's *estimate* in arbitrary units. Useful only for comparing plans, not as a time.
- **rows** (in `cost`) — estimated row count. **rows** (in `actual`) — real row count.
- **actual time=first..last** — real milliseconds to first row .. to last row, **per loop**.
- **loops** — how many times this node ran. **Total time for the node = actual time × loops.** This is the #1 misread — a "fast" node with 10,000 loops is not fast.

## The tells, in order of importance

**1. Estimate vs actual row mismatch.** If estimated `rows=1` but actual `rows=50000`, the planner is working from bad statistics and every downstream choice is suspect. Fix: `ANALYZE <table>;` to refresh stats. This is the root cause behind a surprising share of "the planner picked a dumb plan" cases.

**2. `Seq Scan` on a big table where you expected an index.** The flame-graph-wide bar. Either the index is missing, or a predicate isn't sargable (e.g., `WHERE lower(col) = ...` without a matching functional index, or a type mismatch forcing a cast). Fix: add the index, or make the predicate index-friendly.

**3. The node with the largest `actual time × loops`.** This is your hotspot — the widest bar in the flame graph. Optimize here; ignore the cheap nodes no matter how ugly they look.

**4. Nested Loop with high loop count.** Classic N+1 at the plan level: the inner side runs once per outer row. Fine for small outer sets, catastrophic for large ones. Fix: often a `Hash Join` is what you want — check whether a missing index on the join key forced the nested loop.

**5. `BUFFERS`: high `read` vs `hit`.** `shared hit` = came from cache (fast), `shared read` = came from disk (slow). Lots of `read` means the working set doesn't fit in cache or the query touches far more data than it should.

**6. External sort / `Sort Method: external merge Disk`.** The sort spilled to disk because `work_mem` was too small. Fix: reduce rows sorted (index for ordering), or raise `work_mem` for that workload.

## Quick diagnostic loop

1. Run `EXPLAIN (ANALYZE, BUFFERS)`.
2. Find the node with the biggest `actual time × loops`.
3. Ask: is it a scan that should be an index? A loop that should be a join? A sort that should be pre-ordered by an index? A stats mismatch?
4. Make one change. Re-run. Compare total execution time.
5. Repeat until the hotspot moves somewhere you can't cheaply improve.

## Supporting catalog views

- `pg_stat_statements` — top queries by total time / call count. Start here to find *which* query to `EXPLAIN`.
- `pg_stat_user_indexes` — `idx_scan = 0` means a dead index (write cost, no read benefit).
- `pg_stat_user_tables` — `seq_scan` vs `idx_scan` ratio per table; high seq_scan on a large hot table is a flag.

## Mental model for the transition

A flame graph shows where CPU time goes; a query plan shows where a query's time goes. Same skill: find the widest bar, understand why it's wide, narrow it, re-measure. If you can already read a flame graph, you are most of the way to reading a plan — the vocabulary is just `Seq Scan`/`Nested Loop`/`Sort` instead of function frames.
