//go:build archtest

package arch

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// pureCore packages must stay dependency-clean: the significance math and the
// domain types are testable in microseconds precisely because they touch no I/O
// and no framework (CLAUDE.md §3).
var pureCore = []string{
	"github.com/tomtomtomdev/vantage/internal/domain",
	"github.com/tomtomtomdev/vantage/internal/verdict",
}

// forbidden import prefixes. A match anywhere in a pure-core package's transitive
// import graph is an architecture violation — dependencies point inward, so the
// core cannot know about adapters, the database, the CLI, or the wire.
var forbidden = []struct{ prefix, why string }{
	{"github.com/tomtomtomdev/vantage/internal/adapters", "adapters — the pure core must not know its I/O"},
	{"github.com/jackc/pgx", "pgx — no database in the pure core"},
	{"github.com/spf13/cobra", "cobra — no CLI framework in the pure core"},
	{"net/http", "net/http — no transport in the pure core"},
}

// TestArchDependencyRule loads each pure-core package's full transitive import
// graph via `go list -deps -json` and fails on any forbidden import. We shell out
// to the toolchain rather than depend on golang.org/x/tools/go/packages so the
// module graph of the pure core stays trivially provable (and the test runs
// offline). Swap to go/packages later if a richer query is ever needed.
//
// Always run with -count=1 (make arch does): the verdict depends on the import
// graph read via `go list`, which the build cache cannot observe, so a cached
// pass could mask a fresh violation.
func TestArchDependencyRule(t *testing.T) {
	for _, pkg := range pureCore {
		for _, dep := range transitiveDeps(t, pkg) {
			for _, f := range forbidden {
				if dep == f.prefix || strings.HasPrefix(dep, f.prefix+"/") {
					t.Errorf("dependency rule violated: %s transitively imports %q — %s", pkg, dep, f.why)
				}
			}
		}
	}
}

// transitiveDeps returns every package in pkg's transitive closure (pkg itself
// included). `go list -deps -json` streams one JSON object per package.
func transitiveDeps(t *testing.T, pkg string) []string {
	t.Helper()

	var stderr bytes.Buffer
	cmd := exec.Command("go", "list", "-deps", "-json", pkg)
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps -json %s: %v\n%s", pkg, err, stderr.String())
	}

	var paths []string
	dec := json.NewDecoder(bytes.NewReader(stdout))
	for dec.More() {
		var p struct{ ImportPath string }
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decoding go list output for %s: %v", pkg, err)
		}
		paths = append(paths, p.ImportPath)
	}
	return paths
}
