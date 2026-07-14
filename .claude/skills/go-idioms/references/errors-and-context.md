# Errors and Context — values, wrapping, and cancellation

Two Go idioms that engineers from exception-based languages get subtly wrong.
Errors are ordinary values you return and inspect, not control flow you throw.
Context is how cancellation and deadlines travel — get it wrong and work runs on
after the caller has given up.

## Errors are values

- **Return them, don't swallow them.** `_ = doThing()` and unchecked returns are
  the most common Go bug. Turn on `errcheck` (via `golangci-lint`) so ignoring an
  error is a lint failure, not a silent choice.
- **Wrap with `%w` to preserve the chain:**
  ```go
  if err != nil {
      return fmt.Errorf("saving run %s: %w", id, err)
  }
  ```
  `%w` keeps the underlying error inspectable; `%v` flattens it to a string and
  loses that. Add context at each layer (what you were doing), not a restatement of
  the error.
- **Inspect with `errors.Is` / `errors.As`, not string matching:**
  - `errors.Is(err, ErrNotFound)` — is this (or does it wrap) a known sentinel?
  - `errors.As(err, &myErr)` — is there a typed error of this shape in the chain?
- **Sentinel vs typed:**
  - *Sentinel* (`var ErrNotFound = errors.New("not found")`) for "which condition"
    with no data.
  - *Typed* (`type ValidationError struct{ Field string }` implementing `Error()`)
    when the caller needs data off the error. Define domain errors as values in the
    domain package so callers can branch on them without importing an adapter.
- **No `panic` in library code.** Panic is for programmer bugs (impossible states),
  not for expected failures like "row not found" or "request timed out." A library
  that panics on bad input is a library that takes down its caller. `main`/`cmd`
  may `log.Fatal` at the very top; nothing below should.
- **Log once, at the boundary.** Wrapping-and-returning at each layer plus logging
  once where the error stops (the HTTP handler, the CLI command) gives one clear
  line with the full chain. Logging at every layer produces the same error five
  times and hides the real one.

## Context: cancellation and deadlines

- **`ctx context.Context` is the first parameter** of any function that does I/O,
  blocks, or spawns work. This is a hard convention — follow it so your code
  composes with everyone else's.
- **Never store a context in a struct.** It's request-scoped; a struct outlives the
  request. `contextcheck` and reviewers will flag a `ctx` field. Pass it per call.
  (Rare documented exceptions exist; treat them as exceptions.)
- **Propagate it down.** If you receive a `ctx`, pass *that one* (or a derived
  child) onward — don't start a fresh `context.Background()` mid-call and sever the
  cancellation chain.
- **`Background` vs `TODO`:** `context.Background()` at the true top (main, test
  root, request entry). `context.TODO()` as a marker that a context *should* be
  threaded here but isn't yet — a visible loose end, not a permanent choice.
- **Timeout every outbound call.** Network and DB calls get
  `ctx, cancel := context.WithTimeout(parent, d); defer cancel()`. An un-timed
  outbound call is an unbounded latency and a resource leak under failure — this is
  also a `backend-audit-refactor` perf rule (synchronous fan-out with no timeouts).
- **`defer cancel()` always.** `WithTimeout`/`WithCancel` return a `cancel` you
  must call, even on the success path, or you leak the context's resources. `go vet`
  catches a lost cancel.
- **Honor it.** Receiving a ctx and never selecting on `ctx.Done()` / never passing
  it to the calls that accept it means cancellation does nothing — the work grinds
  on after the caller has walked away. This is "swallowed cancellation," and it's
  invisible until something hangs under load.

## Checklist

- [ ] `errcheck` on — no silently ignored errors.
- [ ] Errors wrapped with `%w`, adding what-you-were-doing context per layer.
- [ ] Inspected with `errors.Is`/`As`; domain errors are values in the domain pkg.
- [ ] No `panic` below `cmd/`; log once at the boundary.
- [ ] `ctx` is the first arg of I/O/blocking/spawning funcs; never a struct field.
- [ ] Context propagated, not reset mid-chain; `defer cancel()` on every derive.
- [ ] Every outbound I/O call carries a timeout and actually honors `ctx.Done()`.
