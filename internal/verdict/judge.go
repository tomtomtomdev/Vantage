package verdict

import (
	"errors"

	"github.com/tomtomtomdev/vantage/internal/domain"
)

// Status is a single metric's PASS/FAIL outcome against its SLO.
type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
)

// ErrNoSLO refuses to render a verdict with nothing declared. Number-before-narrative,
// correctly scoped (SPEC §8): a PASS/FAIL needs a declared SLO — an empty set is not
// "all pass", it is "you haven't said what good looks like".
var ErrNoSLO = errors.New("judge: no SLO declared — cannot render PASS/FAIL without a threshold")

// MetricVerdict is one SLO's outcome: the SLO as declared, the Run's actual value
// (in the SLO's unit), and PASS/FAIL. It carries the SLO so the caller can render
// the full "p99 <= 150ms: 182 → FAIL" line without re-deriving anything.
type MetricVerdict struct {
	SLO    domain.SLO
	Actual float64
	Status Status
}

// Verdicts is the per-metric result of judging one Run against its SLOs.
type Verdicts []MetricVerdict

// AllPass reports whether every declared SLO was met. Empty ⇒ false (there is
// nothing to pass); callers that want a verdict must go through Judge, which
// refuses an empty SLO set outright.
func (vs Verdicts) AllPass() bool {
	if len(vs) == 0 {
		return false
	}
	for _, v := range vs {
		if v.Status != Pass {
			return false
		}
	}
	return true
}

// Judge renders a per-metric PASS/FAIL of a Run's pooled Result against its
// declared SLOs (SPEC §7). Pure and baseline-free — this is the verdict flavour
// that needs only an SLO. Returns ErrNoSLO if none is declared.
func Judge(r domain.Result, slos []domain.SLO) (Verdicts, error) {
	if len(slos) == 0 {
		return nil, ErrNoSLO
	}
	out := make(Verdicts, 0, len(slos))
	for _, s := range slos {
		actual, pass := s.Check(r)
		status := Fail
		if pass {
			status = Pass
		}
		out = append(out, MetricVerdict{SLO: s, Actual: actual, Status: status})
	}
	return out, nil
}
