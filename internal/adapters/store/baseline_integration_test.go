//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"plumber/internal/adapters/store"
	"plumber/internal/domain"
	"plumber/internal/hist"
)

// saveOneRun persists a small Run and returns (targetID, runID) for the
// GetRun/baseline round-trips below.
func saveOneRun(t *testing.T, s *store.PgStore, targetName string, centersMs ...float64) (int64, int64) {
	t.Helper()
	ctx := context.Background()

	tg, err := domain.NewTarget(targetName, "http://localhost:8080", domain.ModeWhiteBox, domain.WithAllowlisted())
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	targetID, err := s.AddTarget(ctx, tg)
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}

	p, err := domain.NewConstantProfile(100, 60*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	reps := make([]domain.RepResult, len(centersMs))
	for i, c := range centersMs {
		h := hist.New()
		for j := 0; j < 500; j++ {
			_ = h.RecordDuration(time.Duration(c * float64(time.Millisecond)))
		}
		blob, err := h.Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		reps[i] = domain.RepResult{Seq: i + 1, Histogram: blob, P99Ms: c, MaxMs: c, AchievedRPS: 99}
	}
	run := domain.Run{
		Profile: p, NReps: len(centersMs), VersionMarker: "sha-1", Label: "before-fix",
		Env:  domain.EnvFingerprint{Host: "h1", Colocation: "same-host", Extra: map[string]string{"positions_rows": "1000000"}},
		Reps: reps, DriverOverheadMs: 0.5, StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now(),
	}
	runID, err := s.SaveRun(ctx, targetID, run)
	if err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	return targetID, runID
}

func TestGetRun_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))
	targetID, runID := saveOneRun(t, s, "sluice-get", 150, 155, 160)

	got, gotTargetID, err := s.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotTargetID != targetID {
		t.Errorf("targetID = %d, want %d", gotTargetID, targetID)
	}
	if got.NReps != 3 || len(got.Reps) != 3 {
		t.Fatalf("reps = %d (n_reps %d), want 3", len(got.Reps), got.NReps)
	}
	if got.VersionMarker != "sha-1" || got.Label != "before-fix" {
		t.Errorf("markers = %q/%q, want sha-1/before-fix", got.VersionMarker, got.Label)
	}
	if got.Env.Host != "h1" || got.Env.Extra["positions_rows"] != "1000000" {
		t.Errorf("env = %+v, want host h1 + positions_rows 1000000", got.Env)
	}
	if got.Profile.RPS != 100 || got.Profile.Warmup != 10*time.Second {
		t.Errorf("profile = %+v, want rps 100 warmup 10s", got.Profile)
	}
	// The histogram blob must decode and yield the recorded percentile.
	h, err := hist.Decode(got.Reps[0].Histogram)
	if err != nil {
		t.Fatalf("decode rep histogram: %v", err)
	}
	if p99 := h.PercentileMillis(99); p99 < 140 || p99 > 170 {
		t.Errorf("decoded rep p99 = %.1fms, want ~150", p99)
	}

	if _, _, err := s.GetRun(ctx, 999999); !errors.Is(err, domain.ErrRunNotFound) {
		t.Errorf("missing run err = %v, want domain.ErrRunNotFound", err)
	}
}

func TestBaseline_SetGetAndMove(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))

	// No baseline yet.
	if _, ok, err := s.Baseline(ctx, 1234); err != nil || ok {
		t.Fatalf("Baseline(unset) = ok %v, err %v; want ok=false, no error", ok, err)
	}

	targetID, firstRun := saveOneRun(t, s, "sluice-base", 200, 201, 202)
	if err := s.SetBaseline(ctx, targetID, firstRun); err != nil {
		t.Fatalf("SetBaseline: %v", err)
	}
	got, ok, err := s.Baseline(ctx, targetID)
	if err != nil || !ok || got != firstRun {
		t.Fatalf("Baseline = (%d, %v, %v), want (%d, true, nil)", got, ok, err, firstRun)
	}

	// Setting again moves the pointer (upsert, not a second row).
	_, secondRun := saveOneRunForTarget(t, s, targetID, 160, 161, 162)
	if err := s.SetBaseline(ctx, targetID, secondRun); err != nil {
		t.Fatalf("SetBaseline (move): %v", err)
	}
	got, _, _ = s.Baseline(ctx, targetID)
	if got != secondRun {
		t.Errorf("baseline after move = %d, want %d", got, secondRun)
	}
}

// saveOneRunForTarget persists an additional run against an existing target (so the
// baseline-move test doesn't re-add the target).
func saveOneRunForTarget(t *testing.T, s *store.PgStore, targetID int64, centersMs ...float64) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	p, _ := domain.NewConstantProfile(100, 60*time.Second, 10*time.Second)
	reps := make([]domain.RepResult, len(centersMs))
	for i, c := range centersMs {
		h := hist.New()
		for j := 0; j < 500; j++ {
			_ = h.RecordDuration(time.Duration(c * float64(time.Millisecond)))
		}
		blob, _ := h.Encode()
		reps[i] = domain.RepResult{Seq: i + 1, Histogram: blob}
	}
	run := domain.Run{
		Profile: p, NReps: len(centersMs), Label: "after-fix",
		Env:  domain.EnvFingerprint{Host: "h1", Colocation: "same-host"},
		Reps: reps, StartedAt: time.Now(), FinishedAt: time.Now(),
	}
	runID, err := s.SaveRun(ctx, targetID, run)
	if err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	return targetID, runID
}
