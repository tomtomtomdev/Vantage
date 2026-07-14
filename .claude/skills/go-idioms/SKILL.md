---
name: go-idioms
description: >-
  A correctness reference for writing and reviewing Go backend code — the idioms
  that are easy to get subtly wrong coming from another language (concurrency &
  goroutine lifecycle, context/cancellation, error handling, and hexagonal/Clean
  Architecture the Go way). Use this when writing or reviewing Go services,
  designing package boundaries, spawning goroutines or building worker pools,
  wiring dependencies, or auditing Go code for races and leaks. Trigger it even
  when the code looks fine — "this Go service is done", "quick goroutine to
  handle X", "just wrap this in a channel" — because Go's sharpest bugs (goroutine
  leaks, data races, swallowed cancellation) compile cleanly and pass a casual
  read. Especially for engineers strong in another stack (Swift/iOS, etc.) moving
  to Go, where the concepts transfer but the idioms don't. This is a reference for
  catching mistakes and knowing the tooling that catches them — not a tutorial
  that writes the code for you.
---

# Go Idioms — a correctness reference

## What this is (and isn't)

This is a checklist of the Go-specific ways to be *confidently wrong* — code that
compiles, passes a casual review, and is broken. It is scoped as a **correctness
reference**: it tells you what to watch for and which tool catches it, not how to
implement a worker pool step by step. If you're building something to learn the
concurrency (e.g. by hand, on purpose), this keeps you honest without doing the
thinking for you.

The premise: for an engineer strong in another stack, the *concepts* transfer
(concurrency, dependency inversion, error handling) but Go's *idioms* don't. Swift
actors ≠ goroutines+channels; Swift's `throws` ≠ Go's errors-as-values; protocol
witnesses ≠ Go's structural interfaces. The gap is where the bugs live.

## The tooling that catches most of it (set this up first)

Go's best feature for correctness is that the worst bugs are *detectable* if you
run the right tools. Non-negotiable:

- **`go test -race ./...`** on every run. The race detector finds data races you
  will never find by reading. If it's not in CI, races ship.
- **`go.uber.org/goleak`** in tests that spawn goroutines — fails the test on a
  leaked goroutine. A leak is a slow resource bug and, in measurement code, a
  correctness bug.
- **`golangci-lint`** with at least `govet`, `errcheck` (catches ignored errors),
  `depguard` (enforces architecture boundaries), `bodyclose`, `contextcheck`.
- **`go vet`** — catches misused `sync.WaitGroup`, bad printf verbs, lost cancel
  funcs. Cheap, always on.

If these run, most of the sins below fail loudly instead of silently. That's the
point of the reference — know the sin, lean on the tool.

## The Go correctness sins (scan first)

Each compiles and reads fine. Tell = how to catch it. Ranked by how often they
bite.

1. **Goroutine with no owner or exit path.** Fire-and-forget `go f()` that can
   block forever (send on a channel nobody reads, receive that never comes).
   *Tell:* a `go` statement with no corresponding wait/cancel. *Fix:* every
   goroutine has an owner that waits and a `ctx`-driven exit.
   → `references/concurrency.md`
2. **Data race on shared state.** Two goroutines touch a map/slice/field, one
   writes. *Tell:* `-race` isn't in CI. *Fix:* run `-race`; guard with a mutex or
   don't share — communicate over channels.  → `references/concurrency.md`
3. **Swallowed cancellation.** A blocking loop or I/O call that doesn't select on
   `ctx.Done()` / doesn't pass `ctx` down. *Tell:* a long-running function whose
   first param isn't `ctx`, or an outbound call with no timeout. *Fix:* propagate
   context; timeout every I/O.  → `references/errors-and-context.md`
4. **Ignored error.** `_ = doThing()` or an unchecked return. *Tell:* `errcheck`
   off. *Fix:* handle or wrap-and-return; log once at the boundary.
   → `references/errors-and-context.md`
5. **Interface defined at the implementer, not the consumer.** A package exports a
   big interface "so it's mockable," inverting the dependency. *Tell:* interfaces
   living next to their only implementation. *Fix:* define small interfaces where
   they're *used*.  → `references/architecture.md`
6. **Channel misuse.** Closing a channel from the receiver, closing twice, sending
   on a closed channel (all panic), or a nil channel that blocks forever. *Tell:*
   `close()` outside the sole sender; a `close` in a consumer. *Fix:* sender owns
   the close; one closer.  → `references/concurrency.md`
7. **`context.Context` stored in a struct.** *Tell:* a `ctx` field. *Fix:* pass it
   as the first argument, per call.  → `references/errors-and-context.md`
8. **Over-abstraction / premature DI framework.** Reaching for a DI container or
   deep interface hierarchies. *Tell:* a framework where a constructor in `main`
   would do. *Fix:* wire by hand at a composition root.
   → `references/architecture.md`

## A note the internet will get wrong: loop variables

The classic "loop variable captured by goroutine" bug — `for _, v := range xs { go
func(){ use(v) }() }` all seeing the last `v` — **was fixed in Go 1.22.** Loop
variables are now per-iteration. So on 1.22+ this specific footgun is gone. But:
(a) it still bites on older toolchains, so know it when reading legacy code, and
(b) it never made sharing *other* mutable state safe — the race detector is still
the authority. Don't cargo-cult the old `v := v` shadow on 1.22+; do keep running
`-race`.

## How to use this

- **Writing Go:** set up the tooling above first, then let it catch sins 1–4 for
  you. Consult the reference for the design-time sins (5, 8) where no tool saves
  you — those are review judgment.
- **Reviewing Go:** walk the sins list against the diff. The concurrency ones (1,
  2, 6) are where "looks fine" is most dangerous — if goroutines or channels
  appear and there's no `-race`/`goleak`, that's the review comment.
- **Applied example:** an open-model load driver (owner-per-goroutine, sender-
  closes-channel, ctx-driven abort, `-race` + `goleak`) is the concurrency
  reference in practice — see a project's CLAUDE.md where one is being built.

## References

- `references/concurrency.md` — goroutine lifecycle & leaks, channel ownership,
  `select`/`ctx.Done()`, worker pools, `errgroup`, the race detector & goleak, the
  specific leak/panic patterns.
- `references/errors-and-context.md` — errors as values, `%w`/`Is`/`As`, sentinel
  vs typed errors, no-panic-in-libraries, context as first-arg, cancellation
  propagation, timeouts on I/O.
- `references/architecture.md` — hexagonal/Clean in Go: accept-interfaces-return-
  structs, consumer-defined interfaces, small interfaces, composition root instead
  of a DI framework, `internal/`, package naming, enforcing boundaries with
  `depguard` + an arch-test.
