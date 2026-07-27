package verdict

import (
	"errors"
	"testing"

	"github.com/tomtomtomdev/vantage/internal/domain"
)

func mustSLO(t *testing.T, metric string, threshold float64, unit, comparator string) domain.SLO {
	t.Helper()
	s, err := domain.NewSLO(metric, threshold, unit, comparator, 0)
	if err != nil {
		t.Fatalf("NewSLO(%s): %v", metric, err)
	}
	return s
}

func TestJudge(t *testing.T) {
	// Pooled Result: p99=182ms (> a 150 SLO ⇒ FAIL), error_rate=0.05% (< 0.1 ⇒ PASS),
	// throughput=190rps (>= 150 ⇒ PASS). Proves the generic model spans all three axes.
	r := domain.Result{P99Ms: 182, ErrorRate: 0.0005, ThroughputRPS: 190}
	slos := []domain.SLO{
		mustSLO(t, "p99", 150, "ms", "<="),
		mustSLO(t, "error_rate", 0.1, "%", "<"),
		mustSLO(t, "throughput", 150, "rps", ">="),
	}

	verdicts, err := Judge(r, slos)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if len(verdicts) != 3 {
		t.Fatalf("got %d verdicts, want 3", len(verdicts))
	}

	want := []struct {
		metric domain.MetricKind
		actual float64
		status Status
	}{
		{domain.MetricP99, 182, Fail},
		{domain.MetricErrorRate, 0.05, Pass},
		{domain.MetricThroughput, 190, Pass},
	}
	for i, w := range want {
		v := verdicts[i]
		if v.SLO.Metric != w.metric || v.Actual != w.actual || v.Status != w.status {
			t.Errorf("verdict[%d] = {%s %v %s}, want {%s %v %s}",
				i, v.SLO.Metric, v.Actual, v.Status, w.metric, w.actual, w.status)
		}
	}
}

func TestJudge_AllPass(t *testing.T) {
	r := domain.Result{P99Ms: 100}
	verdicts, err := Judge(r, []domain.SLO{mustSLO(t, "p99", 150, "ms", "<=")})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !verdicts.AllPass() {
		t.Errorf("AllPass() = false, want true for a met SLO")
	}
	if verdicts[0].Status != Pass {
		t.Errorf("status = %s, want PASS", verdicts[0].Status)
	}
}

// The ordering invariant (SPEC §3/§8): a PASS/FAIL verdict cannot be rendered
// without a declared SLO. An empty SLO set is refused, not silently "all pass".
func TestJudge_NoSLO(t *testing.T) {
	_, err := Judge(domain.Result{P99Ms: 1}, nil)
	if !errors.Is(err, ErrNoSLO) {
		t.Fatalf("err = %v, want ErrNoSLO", err)
	}
}

func TestVerdicts_AllPass_FalseOnAnyFail(t *testing.T) {
	r := domain.Result{P99Ms: 300}
	verdicts, err := Judge(r, []domain.SLO{
		mustSLO(t, "p99", 150, "ms", "<="), // FAIL
		mustSLO(t, "p50", 999, "ms", "<="), // PASS
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if verdicts.AllPass() {
		t.Error("AllPass() = true, want false when any metric fails")
	}
}
