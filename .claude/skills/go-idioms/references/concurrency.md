# Concurrency — lifecycle, channels, and the bugs that compile

Go makes concurrency easy to *write* and easy to get *subtly wrong*. The compiler
won't stop you leaking a goroutine or racing a map. This is the reference for the
mistakes that pass review; lean on `-race` and `goleak` for the ones a tool can
catch.

## The one rule: every goroutine has an owner and a guaranteed exit

Before you type `go`, answer two questions: **who waits for this?** and **how does
it stop?** If either has no answer, you have a leak or an orphan.

- **Owner waits.** Use `sync.WaitGroup` (owner calls `Add` *before* the `go`, the
  goroutine `defer wg.Done()`, owner `Wait`s) or `errgroup.Group` (below). Never
  `wg.Add` *inside* the goroutine — the owner may `Wait` before it runs. `go vet`
  catches some of this; not all.
- **Exit path.** A goroutine that can block forever (channel send/recv, network
  read) must also select on `ctx.Done()` so cancellation unblocks it. A goroutine
  whose only exit is "the work finished" leaks the moment the work can't finish.

**Leak patterns to recognize** (all compile, all pass a casual read):

- **Send on a channel nobody will read.** Producer `ch <- x` blocks forever after
  the consumer has gone. Fix: `select { case ch <- x: case <-ctx.Done(): return }`.
- **Receive that never comes.** `<-ch` waiting on a producer that errored out and
  never sent. Fix: producer signals completion (close) even on the error path, or
  ctx cancels the receiver.
- **WaitGroup never reaches zero.** An early `return` skips a `wg.Done()`. Fix:
  `defer wg.Done()` as the goroutine's first line.

**Detect leaks:** add `goleak.VerifyNone(t)` (or `TestMain` with
`goleak.VerifyTestMain`) to any test that spawns goroutines. It fails the test if
a goroutine outlives it — turning an invisible slow bug into a red test.

## Channels: ownership and the panics

- **The sender owns the channel and is the only one who closes it.** Closing from
  a receiver is a category error. Closing a channel signals "no more values" — that
  is a producer statement.
- **Three ways to panic**, all at runtime, none at compile time: close a closed
  channel, close a nil channel, send on a closed channel. If multiple goroutines
  might close, you've modeled it wrong — funnel to a single closer, or use a
  `sync.Once`, or don't close at all (closing is only needed to signal ranged
  consumers).
- **Nil channel blocks forever** on both send and receive. Occasionally useful
  (disabling a `select` case); usually a bug from a forgotten `make`.
- **Receiving from a closed channel** returns the zero value with `ok == false`
  immediately, forever. `v, ok := <-ch` distinguishes closed from a zero value.
- **Buffered vs unbuffered** is a semantic choice, not a perf tweak: unbuffered =
  a rendezvous (send blocks until receive); buffered = a bounded queue (models a
  real limit, e.g. connection count). Pick deliberately.

## `select` and cancellation

Every blocking loop selects on `ctx.Done()`:

```go
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case job, ok := <-jobs:
        if !ok { return nil } // channel closed by sender
        // ...
    }
}
```

Without the `ctx.Done()` case, cancellation can't reach a goroutine parked on the
other case, and it leaks.

## Worker pools

The common shape: a bounded set of workers ranging over a jobs channel, an owner
that closes `jobs` when done and waits for the workers. The bound is the point —
it models the real concurrency limit. Two correctness notes specific to Go:

- The owner closes `jobs` (sender-owns-close). Workers `range jobs` and exit
  naturally when it's closed — *or* on `ctx.Done()`, whichever first.
- If you fan results back on a `results` channel, someone must drain it or the
  workers block on send forever (a leak). Size/drain it deliberately.

For measurement-style pools where arrival timing matters (a load driver), the
arrival schedule must be **independent of worker completion** — that's a
methodology requirement, not just a Go one; see the `perf-measurement-rigor` skill
(`measuring-cleanly.md`, coordinated omission).

## errgroup — the usual right answer

`golang.org/x/sync/errgroup` handles owner-waits + first-error + cancellation in
one:

```go
g, ctx := errgroup.WithContext(ctx)
for _, task := range tasks {
    task := task // pre-1.22 only; unnecessary on Go 1.22+
    g.Go(func() error { return do(ctx, task) })
}
err := g.Wait() // first non-nil error; ctx is cancelled on first failure
```

`WithContext` cancels the shared ctx when any goroutine errors, so siblings that
respect `ctx.Done()` stop promptly. Use `SetLimit` to bound concurrency.

## Data races: don't reason, detect

A data race is any concurrent access to the same memory where at least one is a
write and there's no synchronization. You *cannot* reliably find these by reading —
memory models are counterintuitive and failures are load-dependent. So:

- **Run `go test -race ./...` in CI.** Treat a race like a failing test.
- Prefer **not sharing**: give each goroutine its own state and communicate results
  over channels ("share memory by communicating"). Where you must share, a
  `sync.Mutex`/`RWMutex` or `sync/atomic` — and the race detector still verifies it.
- Common silent races: concurrent map writes (also panics sometimes), appending to
  a shared slice, a "harmless" shared counter, a struct field read in one goroutine
  and written in another.

## Concurrency checklist

- [ ] Every `go` has an owner that waits and a `ctx`-driven exit.
- [ ] Channels closed only by the sole sender; consumers never close.
- [ ] Every blocking loop selects on `ctx.Done()`.
- [ ] Result channels are drained; no goroutine blocks on a send nobody reads.
- [ ] `go test -race ./...` in CI; `goleak` in goroutine-spawning tests.
- [ ] `errgroup` for the common wait+error+cancel case; `SetLimit` to bound.
