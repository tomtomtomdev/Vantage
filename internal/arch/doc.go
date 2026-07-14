// Package arch holds the belt-and-braces architecture test that enforces the
// dependency rule (CLAUDE.md §3): internal/domain and internal/verdict — the
// pure core — must import no adapter, no pgx, no cobra, no net/http, even
// transitively. depguard (make lint) is the fast gate; this test survives
// lint-config drift.
//
// The test lives behind the `archtest` build tag (see arch_test.go) so it stays
// out of the sub-second unit suite; run it with `make arch`. This doc.go carries
// no build tag on purpose — without it, `go build ./...` would fail with
// "build constraints exclude all Go files in internal/arch".
package arch
