package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/hist"
	"github.com/tomtomtomdev/vantage/internal/verdict"
)

// fakeDriver returns a canned RepResult carrying a real encoded histogram (so
// Summarise can decode it), and counts calls. If failErr is set it fails.
type fakeDriver struct {
	calls   int
	failErr error
	latency time.Duration
}

func (d *fakeDriver) Run(context.Context, domain.Target, domain.LoadProfile) (domain.RepResult, error) {
	d.calls++
	if d.failErr != nil {
		return domain.RepResult{}, d.failErr
	}
	h := hist.New()
	_ = h.RecordDuration(d.latency)
	blob, _ := h.Encode()
	return domain.RepResult{
		Histogram:   blob,
		P99Ms:       float64(d.latency.Milliseconds()),
		AchievedRPS: 100,
		Errors:      map[string]int{"500": 1},
	}, nil
}

type fakeResultStore struct {
	savedRun      domain.Run
	savedTargetID int64
	calls         int
}

func (s *fakeResultStore) SaveRun(_ context.Context, targetID int64, run domain.Run) (int64, error) {
	if err := run.Validate(); err != nil {
		return 0, err
	}
	s.calls++
	s.savedRun = run
	s.savedTargetID = targetID
	return 42, nil
}

type fakeProbe struct{ fp domain.EnvFingerprint }

func (p fakeProbe) Fingerprint(context.Context) (domain.EnvFingerprint, error) { return p.fp, nil }

func mustProfile(t *testing.T) domain.LoadProfile {
	t.Helper()
	p, err := domain.NewConstantProfile(100, 10*time.Second, time.Second)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	return p
}

func seededTargets(t *testing.T, opts ...domain.TargetOption) *fakeTargetStore {
	t.Helper()
	tg, err := domain.NewTarget("sluice", "http://localhost:8080", domain.ModeWhiteBox, opts...)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	return &fakeTargetStore{added: []domain.Target{tg}, nextID: 1}
}

func TestRunService_Run(t *testing.T) {
	targets := seededTargets(t, domain.WithAllowlisted())
	driver := &fakeDriver{latency: 20 * time.Millisecond}
	store := &fakeResultStore{}
	probe := fakeProbe{fp: domain.EnvFingerprint{Host: "h", Colocation: "same-vpc"}}
	// Declare a p50<=25ms SLO on target 1: the ~20ms pooled p50 must PASS.
	p50SLO, err := domain.NewSLO("p50", 25, "ms", "<=", 0)
	if err != nil {
		t.Fatalf("NewSLO: %v", err)
	}
	slos := &fakeSLOStore{}
	if _, err := slos.SetSLO(context.Background(), 1, p50SLO); err != nil {
		t.Fatalf("seed SLO: %v", err)
	}

	svc := NewRunService(targets, driver, store, probe, slos)

	res, err := svc.Run(context.Background(), RunParams{
		TargetName: "sluice", Profile: mustProfile(t), NReps: 5,
		VersionMarker: "sha1", Label: "before",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if driver.calls != 5 {
		t.Errorf("driver called %d times, want 5 (one per rep)", driver.calls)
	}
	if store.calls != 1 {
		t.Errorf("SaveRun called %d times, want 1", store.calls)
	}
	if res.RunID != 42 {
		t.Errorf("RunID = %d, want 42", res.RunID)
	}
	if store.savedTargetID != 1 {
		t.Errorf("saved targetID = %d, want 1", store.savedTargetID)
	}

	saved := store.savedRun
	if len(saved.Reps) != 5 || saved.NReps != 5 {
		t.Fatalf("saved %d reps (n_reps=%d), want 5", len(saved.Reps), saved.NReps)
	}
	for i, rep := range saved.Reps {
		if rep.Seq != i+1 {
			t.Errorf("rep[%d].Seq = %d, want %d", i, rep.Seq, i+1)
		}
	}
	if saved.Env.Host != "h" || saved.VersionMarker != "sha1" || saved.Label != "before" {
		t.Errorf("run metadata not carried through: %+v", saved)
	}
	// Pooled p50 reflects the merged 20ms samples.
	if res.Summary.Pooled.P50 < 19 || res.Summary.Pooled.P50 > 21 {
		t.Errorf("pooled p50 = %.2fms, want ~20ms", res.Summary.Pooled.P50)
	}
	// Pooled error rate: each rep has 1 success (histogram) + 1 error ⇒ 5/10.
	if res.Summary.ErrorRate < 0.49 || res.Summary.ErrorRate > 0.51 {
		t.Errorf("pooled error rate = %.3f, want 0.5", res.Summary.ErrorRate)
	}
	// The declared p50<=25ms SLO is judged and passes (pooled p50 ~20ms).
	if len(res.Verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(res.Verdicts))
	}
	if res.Verdicts[0].SLO.Metric != domain.MetricP50 || res.Verdicts[0].Status != verdict.Pass {
		t.Errorf("verdict = %+v, want p50 PASS", res.Verdicts[0])
	}
}

func TestRunService_TargetNotFound(t *testing.T) {
	svc := NewRunService(&fakeTargetStore{}, &fakeDriver{}, &fakeResultStore{}, fakeProbe{}, &fakeSLOStore{})
	_, err := svc.Run(context.Background(), RunParams{TargetName: "ghost", Profile: mustProfile(t), NReps: 3})
	if !errors.Is(err, domain.ErrTargetNotFound) {
		t.Fatalf("err = %v, want domain.ErrTargetNotFound", err)
	}
}

func TestRunService_GuardErrorNotPersisted(t *testing.T) {
	targets := seededTargets(t, domain.WithAllowlisted())
	driver := &fakeDriver{failErr: domain.ErrTargetNotAllowlisted} // driver refuses
	store := &fakeResultStore{}

	svc := NewRunService(targets, driver, store, fakeProbe{}, &fakeSLOStore{})
	_, err := svc.Run(context.Background(), RunParams{TargetName: "sluice", Profile: mustProfile(t), NReps: 3})
	if !errors.Is(err, domain.ErrTargetNotAllowlisted) {
		t.Fatalf("err = %v, want domain.ErrTargetNotAllowlisted", err)
	}
	if store.calls != 0 {
		t.Error("a refused run must not be persisted")
	}
}

func TestRunService_RejectsZeroReps(t *testing.T) {
	targets := seededTargets(t, domain.WithAllowlisted())
	svc := NewRunService(targets, &fakeDriver{}, &fakeResultStore{}, fakeProbe{}, &fakeSLOStore{})
	_, err := svc.Run(context.Background(), RunParams{TargetName: "sluice", Profile: mustProfile(t), NReps: 0})
	if !errors.Is(err, domain.ErrRunInvalid) {
		t.Fatalf("err = %v, want domain.ErrRunInvalid", err)
	}
}
