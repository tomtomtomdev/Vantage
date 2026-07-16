package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// MetricKind names what an SLO constrains (SPEC §3). The latency percentiles are
// read from a Result in ms; error_rate in %; throughput in rps. Generic on purpose
// so the model is not latency-only.
type MetricKind string

const (
	MetricP50        MetricKind = "p50"
	MetricP90        MetricKind = "p90"
	MetricP99        MetricKind = "p99"
	MetricP999       MetricKind = "p99.9"
	MetricMax        MetricKind = "max"
	MetricErrorRate  MetricKind = "error_rate"
	MetricThroughput MetricKind = "throughput"
)

// unit returns the only unit an SLO for this metric may declare, so a threshold's
// number is never ambiguous. ok is false for an unknown metric.
func (m MetricKind) unit() (unit string, ok bool) {
	switch m {
	case MetricP50, MetricP90, MetricP99, MetricP999, MetricMax:
		return "ms", true
	case MetricErrorRate:
		return "%", true
	case MetricThroughput:
		return "rps", true
	default:
		return "", false
	}
}

// Comparator is how an SLO's actual value is tested against its threshold. The set
// matches the slos_comparator_valid CHECK in migrations/0002.
type Comparator string

const (
	CmpLT  Comparator = "<"
	CmpLTE Comparator = "<="
	CmpGT  Comparator = ">"
	CmpGTE Comparator = ">="
	CmpEQ  Comparator = "=="
)

// Valid reports whether c is a known comparator.
func (c Comparator) Valid() bool {
	switch c {
	case CmpLT, CmpLTE, CmpGT, CmpGTE, CmpEQ:
		return true
	default:
		return false
	}
}

// SLO validation errors — typed sentinels, inspected with errors.Is.
var (
	ErrSLOMetricInvalid     = errors.New("slo: unknown metric")
	ErrSLOComparatorInvalid = errors.New("slo: comparator must be one of < <= > >= ==")
	ErrSLOUnitMismatch      = errors.New("slo: unit does not match metric")
	ErrSLOThresholdInvalid  = errors.New("slo: threshold out of range")
)

// SLO is a declared target expressed generically (SPEC §3): {metric, threshold,
// unit, comparator, at_rps}. Declared before a Run and judged against the Run's
// pooled Result. A pure value; NewSLO refuses malformed states at construction so
// every SLO the system holds is well-formed and unit-consistent.
type SLO struct {
	Metric     MetricKind
	Threshold  float64
	Unit       string
	Comparator Comparator
	AtRPS      float64 // the load the SLO applies at; 0 ⇒ unspecified (the "—" in SPEC §3)
}

// NewSLO validates and constructs an SLO. The unit must be the one the metric is
// measured in (ms / % / rps) so a threshold number is never ambiguous.
func NewSLO(metric string, threshold float64, unit, comparator string, atRPS float64) (SLO, error) {
	m := MetricKind(strings.TrimSpace(metric))
	wantUnit, ok := m.unit()
	if !ok {
		return SLO{}, fmt.Errorf("%w: %q", ErrSLOMetricInvalid, metric)
	}

	c := Comparator(strings.TrimSpace(comparator))
	if !c.Valid() {
		return SLO{}, fmt.Errorf("%w: %q", ErrSLOComparatorInvalid, comparator)
	}

	if u := strings.TrimSpace(unit); u != wantUnit {
		return SLO{}, fmt.Errorf("%w: metric %q wants %q, got %q", ErrSLOUnitMismatch, m, wantUnit, u)
	}

	if threshold < 0 {
		return SLO{}, fmt.Errorf("%w: threshold must be >= 0, got %v", ErrSLOThresholdInvalid, threshold)
	}
	if m == MetricErrorRate && threshold > 100 {
		return SLO{}, fmt.Errorf("%w: error_rate %% must be <= 100, got %v", ErrSLOThresholdInvalid, threshold)
	}
	if atRPS < 0 {
		return SLO{}, fmt.Errorf("%w: at_rps must be >= 0, got %v", ErrSLOThresholdInvalid, atRPS)
	}

	return SLO{Metric: m, Threshold: threshold, Unit: wantUnit, Comparator: c, AtRPS: atRPS}, nil
}

// actual extracts the metric's value from r in the SLO's declared unit — the
// single place error_rate is converted from a [0,1] fraction to %.
func (s SLO) actual(r Result) float64 {
	switch s.Metric {
	case MetricP50:
		return r.P50Ms
	case MetricP90:
		return r.P90Ms
	case MetricP99:
		return r.P99Ms
	case MetricP999:
		return r.P999Ms
	case MetricMax:
		return r.MaxMs
	case MetricErrorRate:
		return r.ErrorRate * 100
	case MetricThroughput:
		return r.ThroughputRPS
	default:
		return math.NaN()
	}
}

// Check evaluates the SLO against a pooled Result, returning the actual value (in
// the SLO's unit) and whether the threshold is met. Pure; no baseline needed —
// this is the PASS/FAIL that requires only a declared SLO (SPEC §7/§8).
func (s SLO) Check(r Result) (actual float64, pass bool) {
	a := s.actual(r)
	switch s.Comparator {
	case CmpLT:
		return a, a < s.Threshold
	case CmpLTE:
		return a, a <= s.Threshold
	case CmpGT:
		return a, a > s.Threshold
	case CmpGTE:
		return a, a >= s.Threshold
	case CmpEQ:
		return a, a == s.Threshold
	default:
		return a, false
	}
}
