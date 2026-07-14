# Architecture — hexagonal / Clean, the Go way

The principle (dependencies point inward; the domain knows nothing of I/O) is
language-agnostic. The *idiom* is very Go, and it's where people coming from
framework-heavy stacks over-engineer. Go's structural interfaces make the clean
version simpler than you expect — the trick is to stop adding machinery.

## Accept interfaces, return structs

The core Go proverb, and the whole architecture follows from it:

- A constructor returns a **concrete struct** (`*PgStore`, `*OpenModelDriver`) —
  concrete types are easiest to use, test, and extend.
- A function **accepts an interface** describing only the behavior it needs. The
  caller passes the concrete struct; it satisfies the interface *structurally* (no
  `implements` keyword, no declaration linking them).

This is dependency inversion without ceremony: the high-level code depends on a
small interface it owns, the low-level struct happens to satisfy it.

## Define interfaces at the consumer, not the implementer

The most common Go architecture mistake (sin #5): a `store` package exports a fat
`Store` interface "so it can be mocked," and now the domain imports the store
package to use the interface — the dependency points the wrong way.

Instead: **the package that *uses* a dependency declares the interface it needs.**
The domain/app defines `type ResultStore interface { Save(...) ... }` because it
is the consumer. The `store` adapter imports the domain to implement it (dependency
points inward), and exports a concrete `*PgStore`. The adapter needn't even name
the interface.

Corollary: **keep interfaces small.** One to three methods. A big interface is a
sign it's defined at the implementer (mirroring a struct) rather than at a consumer
(describing a need). Go's stdlib is the model — `io.Reader`, `io.Writer` are one
method. "The bigger the interface, the weaker the abstraction."

## No DI framework — use a composition root

Coming from stacks with DI containers, the instinct is to reach for one. Don't. In
Go, dependency injection *is* passing arguments to constructors, and wiring happens
by hand in one place:

```go
// cmd/app/main.go — the composition root, the ONLY place adapters meet the app
pool  := mustConnect(cfg)
store := store.New(pool)          // concrete
driver:= loaddriver.New(cfg)      // concrete
svc   := app.New(store, driver)   // app.New accepts the small port interfaces
```

Everything below receives its dependencies through constructors. No globals, no
service locator, no `init()` side effects, no reflection-based container. If wiring
in `main` gets unwieldy, that's a signal to group construction into a few helper
functions — still hand-wired, still explicit. Explicit wiring is a feature: you can
read the entire dependency graph in one file.

## Package structure

- **`internal/`** for everything that isn't a deliberate public API — the compiler
  forbids imports from outside the module, so you can't accidentally expose
  internals.
- **One package, one responsibility.** `domain`, `verdict`, `ports`, `app`,
  `adapters/...`. The domain and any pure-logic packages import *nothing* from
  adapters or frameworks.
- **No stutter.** The package name is part of the identifier: `domain.Target`, not
  `domain.DomainTarget`; `store.New`, not `store.NewStore`. Read the call site.
- **Avoid `util`/`common`/`helpers` dumping grounds.** They become import magnets
  and dependency tangles. Name packages for what they *are*.
- **Zero-value-useful** where you can: a struct usable without a constructor (a
  `bytes.Buffer` is the model) is easier and safer than one that must be
  initialized just so.

## Enforce the boundary with a test, not vigilance

An architecture rule that isn't checked erodes. Two cheap enforcers:

- **`depguard`** (a `golangci-lint` linter): deny imports of `adapters/...`,
  `pgx`, `net/http`, web frameworks, etc. from `domain` and `verdict`. Fast, runs
  in CI, fails the build on a violation.
- **An arch-test** as belt-and-braces: a small test using `go/packages` (or
  golang.org/x/tools) that loads the pure packages and asserts their import graph
  contains none of the forbidden prefixes. This survives lint-config drift.

Write these early (they cost little) — the boundary you don't test will have leaked
by the time the codebase is interesting.

## Checklist

- [ ] Constructors return concrete structs; functions accept small interfaces.
- [ ] Interfaces defined where consumed (domain/app owns its ports), 1–3 methods.
- [ ] Dependencies hand-wired at a single composition root; no DI framework/globals.
- [ ] `internal/`; one responsibility per package; no stutter; no `util` dump.
- [ ] Pure packages import no adapters/frameworks — enforced by `depguard` + arch-test.
