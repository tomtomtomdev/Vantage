package app

import (
	"context"
	"errors"
	"testing"

	"plumber/internal/domain"
)

// fakeTargetStore is an in-memory ports.TargetStore: the app layer's use cases
// are unit-tested with no database, so the suite stays sub-second (CLAUDE §5).
type fakeTargetStore struct {
	added   []domain.Target
	nextID  int64
	failErr error // when set, AddTarget returns it (e.g. duplicate)
}

func (f *fakeTargetStore) AddTarget(_ context.Context, t domain.Target) (int64, error) {
	if f.failErr != nil {
		return 0, f.failErr
	}
	f.nextID++
	f.added = append(f.added, t)
	return f.nextID, nil
}

func (f *fakeTargetStore) ListTargets(context.Context) ([]domain.Target, error) {
	return f.added, nil
}

func (f *fakeTargetStore) GetTargetByName(_ context.Context, name string) (domain.Target, int64, error) {
	for i, t := range f.added {
		if t.Name == name {
			return t, int64(i + 1), nil
		}
	}
	return domain.Target{}, 0, domain.ErrTargetNotFound
}

func TestTargetServiceAdd(t *testing.T) {
	t.Run("validates in the domain and persists a well-formed target", func(t *testing.T) {
		store := &fakeTargetStore{}
		svc := NewTargetService(store)

		id, err := svc.Add(context.Background(), "sluice", "http://localhost:8080", "white-box", false, true)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		if id != 1 {
			t.Errorf("id = %d, want 1", id)
		}
		if len(store.added) != 1 {
			t.Fatalf("stored %d targets, want 1", len(store.added))
		}
		got := store.added[0]
		if got.Name != "sluice" || got.Mode != domain.ModeWhiteBox || !got.Allowlisted || got.Mutating {
			t.Errorf("stored %+v, want allowlisted non-mutating white-box sluice", got)
		}
	})

	t.Run("rejects an invalid target before touching the store", func(t *testing.T) {
		store := &fakeTargetStore{}
		svc := NewTargetService(store)

		_, err := svc.Add(context.Background(), "bad", "not-a-url", "black-box", false, false)
		if !errors.Is(err, domain.ErrTargetBaseURL) {
			t.Fatalf("err = %v, want ErrTargetBaseURL", err)
		}
		if len(store.added) != 0 {
			t.Error("an invalid target reached the store; validation must gate persistence")
		}
	})

	t.Run("propagates a store error unchanged", func(t *testing.T) {
		store := &fakeTargetStore{failErr: domain.ErrTargetExists}
		svc := NewTargetService(store)

		_, err := svc.Add(context.Background(), "dup", "http://x.test", "black-box", false, false)
		if !errors.Is(err, domain.ErrTargetExists) {
			t.Fatalf("err = %v, want domain.ErrTargetExists", err)
		}
	})
}
