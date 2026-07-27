package loaddriver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/hist"
)

// goleak turns a leaked goroutine into a red test — a leak in the load driver is
// a measurement bug waiting to happen (CLAUDE §4).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func allowlistedTarget(t *testing.T, url string) domain.Target {
	t.Helper()
	tg, err := domain.NewTarget("stub", url, domain.ModeBlackBox, domain.WithAllowlisted())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	return tg
}

// Coordinated omission is THE cardinal sin (perf-measurement-rigor §1). With a
// slow target and a bounded pool, a closed-loop client would under-report the
// tail because it stops issuing requests while blocked. The open-model driver
// measures from *intended dispatch*, so the queueing delay of arrivals that piled
// up while workers were busy shows up in the tail. We assert exactly that: p99 is
// far above the ~50ms service time.
func TestOpenModel_MeasuresQueueingFromIntendedDispatch(t *testing.T) {
	const serviceTime = 50 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(serviceTime)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{Workers: 2, RequestTimeout: 10 * time.Second})
	defer d.CloseIdleConns()

	// 100rps for 500ms = 50 arrivals; 2 workers × (1000/50)ms ≈ 40 req/s max, so
	// the arrivals pile up and late ones wait ~seconds from their intended time.
	p, err := domain.NewConstantProfile(100, 500*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	rep, err := d.Run(t.Context(), allowlistedTarget(t, srv.URL), p)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if rep.P99Ms < 200 {
		t.Errorf("p99 = %.1fms; want >> service time (%s) — queueing not captured (coordinated omission?)", rep.P99Ms, serviceTime)
	}
	// A saturated pool cannot achieve the requested 100rps; if it "did", we're
	// measuring from send-time and lying.
	if rep.AchievedRPS >= 100 {
		t.Errorf("achieved %.1f rps at a saturated pool; open-model throughput must fall short of requested", rep.AchievedRPS)
	}
}

// Warmup samples come from a cold system and must be discarded (perf sin §5).
// Classification is by intended-dispatch time, so it is deterministic by index:
// a profile with warmup = half the duration measures exactly half the arrivals.
func TestWarmupDiscarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{Workers: 8, RequestTimeout: 5 * time.Second})
	defer d.CloseIdleConns()

	// 100rps × 400ms = 40 arrivals; warmup 200ms discards the first 20.
	p, err := domain.NewConstantProfile(100, 400*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	rep, err := d.Run(t.Context(), allowlistedTarget(t, srv.URL), p)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	h, err := hist.Decode(rep.Histogram)
	if err != nil {
		t.Fatalf("decode histogram: %v", err)
	}
	if got := h.TotalCount(); got != 20 {
		t.Errorf("histogram has %d samples; want 20 (40 arrivals − 20 warmup)", got)
	}
}

// A failed request's latency is meaningless and a fast 5xx must not flatter the
// tail (perf sin §6). Errors are tallied on a separate axis and excluded from the
// latency histogram.
func TestSuccessOnlyLatency_ErrorsTallied(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1)%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{Workers: 8, RequestTimeout: 5 * time.Second})
	defer d.CloseIdleConns()

	p, err := domain.NewConstantProfile(100, 300*time.Millisecond, 0) // 30 arrivals, no warmup
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	rep, err := d.Run(t.Context(), allowlistedTarget(t, srv.URL), p)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	h, err := hist.Decode(rep.Histogram)
	if err != nil {
		t.Fatalf("decode histogram: %v", err)
	}

	var failed int
	for _, c := range rep.Errors {
		failed += c
	}
	measured := int(h.TotalCount()) + failed
	if measured != 30 {
		t.Errorf("accounted for %d measured requests (%d ok + %d failed); want 30", measured, h.TotalCount(), failed)
	}
	if rep.Errors["500"] == 0 {
		t.Errorf("no 500s tallied; errors axis not populated: %v", rep.Errors)
	}
	if rep.ErrorRate < 0.3 || rep.ErrorRate > 0.7 {
		t.Errorf("error rate = %.2f; want ~0.5 with an alternating handler", rep.ErrorRate)
	}
}

// The driver's own overhead is recorded per run; an unsaturated, fast run should
// keep up with its schedule (low overhead, achieved ≈ requested).
func TestDriverOverheadRecorded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{Workers: 16, RequestTimeout: 5 * time.Second})
	defer d.CloseIdleConns()

	p, err := domain.NewConstantProfile(50, 400*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	rep, err := d.Run(t.Context(), allowlistedTarget(t, srv.URL), p)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if rep.DriverOverheadMs < 0 {
		t.Errorf("driver overhead = %.2fms; must be recorded and non-negative", rep.DriverOverheadMs)
	}
	if rep.DriverOverheadMs > 100 {
		t.Errorf("driver overhead = %.2fms; unsaturated run should keep up with its schedule", rep.DriverOverheadMs)
	}
	if rep.AchievedRPS < 40 || rep.AchievedRPS > 60 {
		t.Errorf("achieved %.1f rps; unsaturated run should be ≈ requested 50", rep.AchievedRPS)
	}
}

// Blast-radius guards refuse before any load is generated (SPEC §9). Each is an
// entry-path decision, tested without a server.
func TestGuards(t *testing.T) {
	p, err := domain.NewConstantProfile(100, time.Second, 0)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}

	t.Run("not allowlisted", func(t *testing.T) {
		tg, _ := domain.NewTarget("x", "https://x.internal/y", domain.ModeBlackBox) // no allowlist
		d := New(Config{Workers: 1, RequestTimeout: time.Second})
		if _, err := d.Run(t.Context(), tg, p); !errors.Is(err, domain.ErrTargetNotAllowlisted) {
			t.Fatalf("got %v, want ErrTargetNotAllowlisted", err)
		}
	})

	t.Run("mutating refused", func(t *testing.T) {
		tg, _ := domain.NewTarget("x", "https://x.internal/y", domain.ModeBlackBox,
			domain.WithAllowlisted(), domain.WithMutating())
		d := New(Config{Workers: 1, RequestTimeout: time.Second}) // AllowMutating false
		if _, err := d.Run(t.Context(), tg, p); !errors.Is(err, domain.ErrMutatingRefused) {
			t.Fatalf("got %v, want ErrMutatingRefused", err)
		}
	})

	t.Run("mutating allowed with override", func(t *testing.T) {
		tg, _ := domain.NewTarget("x", "https://x.internal/y", domain.ModeBlackBox,
			domain.WithAllowlisted(), domain.WithMutating())
		d := New(Config{Workers: 1, RequestTimeout: time.Second, AllowMutating: true, MaxRPS: 10})
		// Rate ceiling still applies — 100rps over a ceiling of 10 must refuse.
		if _, err := d.Run(t.Context(), tg, p); !errors.Is(err, domain.ErrRateCeiling) {
			t.Fatalf("got %v, want ErrRateCeiling", err)
		}
	})

	t.Run("rate ceiling", func(t *testing.T) {
		tg, _ := domain.NewTarget("x", "https://x.internal/y", domain.ModeBlackBox, domain.WithAllowlisted())
		d := New(Config{Workers: 1, RequestTimeout: time.Second, MaxRPS: 50})
		if _, err := d.Run(t.Context(), tg, p); !errors.Is(err, domain.ErrRateCeiling) {
			t.Fatalf("got %v, want ErrRateCeiling", err)
		}
	})
}
