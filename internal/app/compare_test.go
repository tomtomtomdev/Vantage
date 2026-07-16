package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"plumber/internal/domain"
	"plumber/internal/hist"
	"plumber/internal/verdict"
)

// storedRun couples a Run to the target it was measured against, as the store holds it.
type storedRun struct {
	run      domain.Run
	targetID int64
}

// fakeBaselineStore is an in-memory ports.BaselineStore for wiring tests — no DB.
type fakeBaselineStore struct {
	runs      map[int64]storedRun
	baselines map[int64]int64 // targetID → runID
	setCalls  int
}

func newFakeBaselineStore() *fakeBaselineStore {
	return &fakeBaselineStore{runs: map[int64]storedRun{}, baselines: map[int64]int64{}}
}

func (f *fakeBaselineStore) GetRun(_ context.Context, runID int64) (domain.Run, int64, error) {
	sr, ok := f.runs[runID]
	if !ok {
		return domain.Run{}, 0, domain.ErrRunNotFound
	}
	return sr.run, sr.targetID, nil
}

func (f *fakeBaselineStore) SetBaseline(_ context.Context, targetID, runID int64) error {
	f.baselines[targetID] = runID
	f.setCalls++
	return nil
}

func (f *fakeBaselineStore) Baseline(_ context.Context, targetID int64) (int64, bool, error) {
	id, ok := f.baselines[targetID]
	return id, ok, nil
}

// buildRun assembles a domain.Run with one single-valued, encoded histogram per
// center — the same fixture shape verdict's tests use, but persisted-encoded so the
// service exercises the decode path.
func buildRun(t *testing.T, centersMs ...float64) domain.Run {
	t.Helper()
	profile, err := domain.NewConstantProfile(100, 60*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("NewConstantProfile: %v", err)
	}
	reps := make([]domain.RepResult, len(centersMs))
	for i, c := range centersMs {
		h := hist.New()
		for j := 0; j < 2000; j++ {
			if err := h.RecordDuration(time.Duration(c * float64(time.Millisecond))); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
		blob, err := h.Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		reps[i] = domain.RepResult{Seq: i + 1, Histogram: blob}
	}
	return domain.Run{
		Profile: profile,
		NReps:   len(centersMs),
		Env:     domain.EnvFingerprint{Host: "h1", Colocation: "same-host"},
		Reps:    reps,
	}
}

func TestCompareService_SetBaseline(t *testing.T) {
	st := newFakeBaselineStore()
	st.runs[7] = storedRun{run: buildRun(t, 200, 201, 202), targetID: 42}
	svc := NewCompareService(st)

	targetID, err := svc.SetBaseline(context.Background(), 7)
	if err != nil {
		t.Fatalf("SetBaseline: %v", err)
	}
	if targetID != 42 {
		t.Errorf("targetID = %d, want 42", targetID)
	}
	if st.baselines[42] != 7 {
		t.Errorf("baseline for target 42 = %d, want run 7", st.baselines[42])
	}
}

func TestCompareService_SetBaseline_RunNotFound(t *testing.T) {
	svc := NewCompareService(newFakeBaselineStore())
	if _, err := svc.SetBaseline(context.Background(), 999); !errors.Is(err, domain.ErrRunNotFound) {
		t.Errorf("err = %v, want ErrRunNotFound", err)
	}
}

func TestCompareService_Compare_KnownShift_Significant(t *testing.T) {
	st := newFakeBaselineStore()
	st.runs[1] = storedRun{run: buildRun(t, 198, 199, 200, 201, 202), targetID: 42} // baseline
	st.runs[2] = storedRun{run: buildRun(t, 158, 159, 160, 161, 162), targetID: 42} // candidate
	st.baselines[42] = 1
	svc := NewCompareService(st)

	res, err := svc.Compare(context.Background(), 2, verdict.WithSeed(1))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.Comparison.Comparability != verdict.Comparable {
		t.Fatalf("comparability = %q, want comparable (%s)", res.Comparison.Comparability, res.Comparison.Reason)
	}
	if res.BaselineRunID != 1 || res.CandidateRunID != 2 {
		t.Errorf("run ids = base %d cand %d, want 1/2", res.BaselineRunID, res.CandidateRunID)
	}
	var sawP99 bool
	for _, d := range res.Comparison.Deltas {
		if d.Metric == domain.MetricP99 {
			sawP99 = true
			if d.Significance != verdict.Significant || d.PointMs >= 0 {
				t.Errorf("p99 delta = %+v, want significant improvement", d)
			}
		}
	}
	if !sawP99 {
		t.Error("no p99 delta returned")
	}
}

func TestCompareService_Compare_NoBaseline(t *testing.T) {
	st := newFakeBaselineStore()
	st.runs[2] = storedRun{run: buildRun(t, 158, 159, 160), targetID: 42}
	svc := NewCompareService(st)

	if _, err := svc.Compare(context.Background(), 2); !errors.Is(err, domain.ErrNoBaseline) {
		t.Errorf("err = %v, want ErrNoBaseline", err)
	}
}

func TestCompareService_Compare_CandidateNotFound(t *testing.T) {
	svc := NewCompareService(newFakeBaselineStore())
	if _, err := svc.Compare(context.Background(), 404); !errors.Is(err, domain.ErrRunNotFound) {
		t.Errorf("err = %v, want ErrRunNotFound", err)
	}
}
