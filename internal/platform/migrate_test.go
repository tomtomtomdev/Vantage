package platform

import (
	"errors"
	"testing"
	"testing/fstest"
)

// The runner's ordering and validation are pure over an fs.FS, so they get a
// fast unit test with no container — the DB apply path is covered by the
// integration test (migrate_integration_test.go, build tag).
func TestParseMigrations(t *testing.T) {
	t.Run("returns files in numeric order regardless of map iteration", func(t *testing.T) {
		fsys := fstest.MapFS{
			"0003_c.sql": {Data: []byte("SELECT 3;")},
			"0001_a.sql": {Data: []byte("SELECT 1;")},
			"0002_b.sql": {Data: []byte("SELECT 2;")},
		}
		migs, err := parseMigrations(fsys)
		if err != nil {
			t.Fatalf("parseMigrations: %v", err)
		}
		want := []string{"0001", "0002", "0003"}
		if len(migs) != len(want) {
			t.Fatalf("got %d migrations, want %d", len(migs), len(want))
		}
		for i, w := range want {
			if migs[i].version != w {
				t.Errorf("position %d: version = %q, want %q", i, migs[i].version, w)
			}
		}
	})

	t.Run("rejects a filename without a version prefix", func(t *testing.T) {
		fsys := fstest.MapFS{"create_targets.sql": {Data: []byte("SELECT 1;")}}
		_, err := parseMigrations(fsys)
		if !errors.Is(err, ErrMigrationOrder) {
			t.Fatalf("err = %v, want ErrMigrationOrder", err)
		}
	})

	t.Run("rejects an empty migration file", func(t *testing.T) {
		fsys := fstest.MapFS{"0001_empty.sql": {Data: []byte("   \n")}}
		_, err := parseMigrations(fsys)
		if !errors.Is(err, ErrMigrationOrder) {
			t.Fatalf("err = %v, want ErrMigrationOrder", err)
		}
	})
}
