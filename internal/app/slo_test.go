package app

import (
	"context"
	"errors"
	"testing"

	"plumber/internal/domain"
)

// fakeSLOStore is an in-memory ports.SLOStore keyed by target id, so the app's
// SLO use cases and the run-judging path are unit-tested with no database.
type fakeSLOStore struct {
	byTarget map[int64][]domain.SLO
	failErr  error
}

func (f *fakeSLOStore) SetSLO(_ context.Context, targetID int64, slo domain.SLO) (int64, error) {
	if f.failErr != nil {
		return 0, f.failErr
	}
	if f.byTarget == nil {
		f.byTarget = map[int64][]domain.SLO{}
	}
	// "set": replace any prior SLO for the same metric.
	kept := f.byTarget[targetID][:0:0]
	for _, s := range f.byTarget[targetID] {
		if s.Metric != slo.Metric {
			kept = append(kept, s)
		}
	}
	f.byTarget[targetID] = append(kept, slo)
	return int64(len(f.byTarget[targetID])), nil
}

func (f *fakeSLOStore) SLOsForTarget(_ context.Context, targetID int64) ([]domain.SLO, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	return f.byTarget[targetID], nil
}

func TestSLOServiceSet(t *testing.T) {
	t.Run("validates in the domain and declares against the resolved target", func(t *testing.T) {
		targets := seededTargets(t, domain.WithAllowlisted())
		slos := &fakeSLOStore{}
		svc := NewSLOService(targets, slos)

		id, err := svc.Set(context.Background(), "sluice", "p99", 150, "ms", "<=", 200)
		if err != nil {
			t.Fatalf("Set: %v", err)
		}
		if id != 1 {
			t.Errorf("id = %d, want 1", id)
		}
		got := slos.byTarget[1]
		if len(got) != 1 || got[0].Metric != domain.MetricP99 || got[0].Threshold != 150 {
			t.Fatalf("stored %+v, want one p99<=150 SLO", got)
		}
	})

	t.Run("rejects an invalid SLO before touching the store", func(t *testing.T) {
		targets := seededTargets(t, domain.WithAllowlisted())
		slos := &fakeSLOStore{}
		svc := NewSLOService(targets, slos)

		_, err := svc.Set(context.Background(), "sluice", "p99", 1, "s", "<=", 0) // wrong unit
		if !errors.Is(err, domain.ErrSLOUnitMismatch) {
			t.Fatalf("err = %v, want ErrSLOUnitMismatch", err)
		}
		if len(slos.byTarget) != 0 {
			t.Error("an invalid SLO reached the store; validation must gate persistence")
		}
	})

	t.Run("unknown target surfaces ErrTargetNotFound", func(t *testing.T) {
		svc := NewSLOService(&fakeTargetStore{}, &fakeSLOStore{})
		_, err := svc.Set(context.Background(), "ghost", "p99", 150, "ms", "<=", 0)
		if !errors.Is(err, domain.ErrTargetNotFound) {
			t.Fatalf("err = %v, want ErrTargetNotFound", err)
		}
	})
}
