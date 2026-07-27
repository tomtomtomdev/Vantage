//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tomtomtomdev/vantage/internal/adapters/store"
	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/hist"
)

func TestGetTargetByName(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))

	tg, err := domain.NewTarget("sluice-ohlc", "http://localhost:8080/ohlc", domain.ModeWhiteBox, domain.WithAllowlisted())
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	wantID, err := s.AddTarget(ctx, tg)
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}

	got, gotID, err := s.GetTargetByName(ctx, "sluice-ohlc")
	if err != nil {
		t.Fatalf("GetTargetByName: %v", err)
	}
	if gotID != wantID {
		t.Errorf("id = %d, want %d", gotID, wantID)
	}
	if got.Name != tg.Name || got.BaseURL != tg.BaseURL || got.Mode != tg.Mode || !got.Allowlisted {
		t.Errorf("target = %+v, want %+v", got, tg)
	}

	if _, _, err := s.GetTargetByName(ctx, "nope"); !errors.Is(err, domain.ErrTargetNotFound) {
		t.Errorf("missing target err = %v, want domain.ErrTargetNotFound", err)
	}
}

func TestSaveRunPersistsRepsAndFingerprint(t *testing.T) {
	ctx := context.Background()
	pool := migratedPool(t)
	s := store.New(pool)

	tg, _ := domain.NewTarget("sluice", "http://localhost:8080", domain.ModeWhiteBox, domain.WithAllowlisted())
	targetID, err := s.AddTarget(ctx, tg)
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}

	p, err := domain.NewConstantProfile(100, 10*time.Second, time.Second)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	reps := make([]domain.RepResult, 3)
	for i := range reps {
		h := hist.New()
		_ = h.RecordDuration(time.Duration(10+i) * time.Millisecond)
		blob, err := h.Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		reps[i] = domain.RepResult{
			Seq: i + 1, Histogram: blob,
			P50Ms: 10, P99Ms: 12, MaxMs: 15,
			AchievedRPS: 99.5, ErrorRate: 0.01,
			Errors: map[string]int{"500": 2},
		}
	}

	run := domain.Run{
		Profile:          p,
		NReps:            3,
		VersionMarker:    "abc123",
		Label:            "before-fix",
		Env:              domain.EnvFingerprint{Host: "test-host", Colocation: "same-vpc"},
		Reps:             reps,
		DriverOverheadMs: 0.4,
		StartedAt:        time.Now().Add(-time.Minute),
		FinishedAt:       time.Now(),
	}

	runID, err := s.SaveRun(ctx, targetID, run)
	if err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if runID <= 0 {
		t.Fatalf("runID = %d, want > 0", runID)
	}

	// Reps landed, count matches n_reps.
	var nReps int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reps WHERE run_id = $1`, runID).Scan(&nReps); err != nil {
		t.Fatalf("counting reps: %v", err)
	}
	if nReps != 3 {
		t.Errorf("persisted %d reps, want 3", nReps)
	}

	// env_fingerprint round-trips as jsonb.
	var host string
	if err := pool.QueryRow(ctx, `SELECT env_fingerprint->>'host' FROM runs WHERE id = $1`, runID).Scan(&host); err != nil {
		t.Fatalf("reading fingerprint: %v", err)
	}
	if host != "test-host" {
		t.Errorf("env_fingerprint host = %q, want test-host", host)
	}

	// A Run with n_reps=1 is legal (no-significance, but storable).
	one := run
	one.NReps = 1
	one.Reps = reps[:1]
	if _, err := s.SaveRun(ctx, targetID, one); err != nil {
		t.Errorf("SaveRun n_reps=1 should be legal: %v", err)
	}

	// A malformed Run (rep count mismatch) is refused before touching the DB.
	bad := run
	bad.NReps = 5
	if _, err := s.SaveRun(ctx, targetID, bad); !errors.Is(err, domain.ErrRunInvalid) {
		t.Errorf("SaveRun with mismatched reps err = %v, want domain.ErrRunInvalid", err)
	}
}
