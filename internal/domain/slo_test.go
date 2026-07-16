package domain

import (
	"errors"
	"testing"
)

func TestNewSLO(t *testing.T) {
	tests := []struct {
		name       string
		metric     string
		threshold  float64
		unit       string
		comparator string
		atRPS      float64
		wantErr    error
	}{
		{name: "latency p99 in ms", metric: "p99", threshold: 150, unit: "ms", comparator: "<=", atRPS: 200},
		{name: "latency p99.9 in ms", metric: "p99.9", threshold: 400, unit: "ms", comparator: "<"},
		{name: "error_rate in %", metric: "error_rate", threshold: 0.1, unit: "%", comparator: "<", atRPS: 200},
		{name: "throughput in rps", metric: "throughput", threshold: 200, unit: "rps", comparator: ">="},
		{name: "at_rps optional (zero)", metric: "p50", threshold: 50, unit: "ms", comparator: "<="},

		{name: "unknown metric", metric: "p95", threshold: 1, unit: "ms", comparator: "<", wantErr: ErrSLOMetricInvalid},
		{name: "unknown comparator", metric: "p99", threshold: 1, unit: "ms", comparator: "~", wantErr: ErrSLOComparatorInvalid},
		{name: "unit mismatch: latency not ms", metric: "p99", threshold: 1, unit: "s", comparator: "<", wantErr: ErrSLOUnitMismatch},
		{name: "unit mismatch: error_rate not %", metric: "error_rate", threshold: 1, unit: "ms", comparator: "<", wantErr: ErrSLOUnitMismatch},
		{name: "unit mismatch: throughput not rps", metric: "throughput", threshold: 1, unit: "ms", comparator: ">", wantErr: ErrSLOUnitMismatch},
		{name: "negative threshold", metric: "p99", threshold: -1, unit: "ms", comparator: "<", wantErr: ErrSLOThresholdInvalid},
		{name: "error_rate above 100%", metric: "error_rate", threshold: 101, unit: "%", comparator: "<", wantErr: ErrSLOThresholdInvalid},
		{name: "negative at_rps", metric: "p99", threshold: 1, unit: "ms", comparator: "<", atRPS: -5, wantErr: ErrSLOThresholdInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slo, err := NewSLO(tt.metric, tt.threshold, tt.unit, tt.comparator, tt.atRPS)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if slo.Metric != MetricKind(tt.metric) || slo.Comparator != Comparator(tt.comparator) {
				t.Errorf("stored %+v, want metric %q comparator %q", slo, tt.metric, tt.comparator)
			}
		})
	}
}

func TestSLO_Check(t *testing.T) {
	// A pooled Result the SLOs are judged against. p99=182ms, error_rate=0.05%, throughput=190rps.
	r := Result{P50Ms: 20, P90Ms: 90, P99Ms: 182, P999Ms: 350, MaxMs: 500, ErrorRate: 0.0005, ThroughputRPS: 190}

	tests := []struct {
		name       string
		metric     string
		threshold  float64
		unit       string
		comparator string
		wantActual float64
		wantPass   bool
	}{
		{name: "p99 <= 150 fails (182 > 150)", metric: "p99", threshold: 150, unit: "ms", comparator: "<=", wantActual: 182, wantPass: false},
		{name: "p99 <= 200 passes", metric: "p99", threshold: 200, unit: "ms", comparator: "<=", wantActual: 182, wantPass: true},
		{name: "p50 < 25 passes", metric: "p50", threshold: 25, unit: "ms", comparator: "<", wantActual: 20, wantPass: true},
		{name: "max <= 500 passes (boundary)", metric: "max", threshold: 500, unit: "ms", comparator: "<=", wantActual: 500, wantPass: true},
		{name: "error_rate < 0.1% passes (0.05 < 0.1)", metric: "error_rate", threshold: 0.1, unit: "%", comparator: "<", wantActual: 0.05, wantPass: true},
		{name: "error_rate < 0.01% fails", metric: "error_rate", threshold: 0.01, unit: "%", comparator: "<", wantActual: 0.05, wantPass: false},
		{name: "throughput >= 200 fails (190 < 200)", metric: "throughput", threshold: 200, unit: "rps", comparator: ">=", wantActual: 190, wantPass: false},
		{name: "throughput >= 150 passes", metric: "throughput", threshold: 150, unit: "rps", comparator: ">=", wantActual: 190, wantPass: true},
		{name: "p99 > 100 passes", metric: "p99", threshold: 100, unit: "ms", comparator: ">", wantActual: 182, wantPass: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slo, err := NewSLO(tt.metric, tt.threshold, tt.unit, tt.comparator, 0)
			if err != nil {
				t.Fatalf("NewSLO: %v", err)
			}
			actual, pass := slo.Check(r)
			if actual != tt.wantActual {
				t.Errorf("actual = %v, want %v", actual, tt.wantActual)
			}
			if pass != tt.wantPass {
				t.Errorf("pass = %v, want %v", pass, tt.wantPass)
			}
		})
	}
}
