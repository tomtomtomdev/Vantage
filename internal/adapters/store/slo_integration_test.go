//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/tomtomtomdev/vantage/internal/adapters/store"
	"github.com/tomtomtomdev/vantage/internal/domain"
)

func TestSetAndListSLOs(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))

	tg, err := domain.NewTarget("sluice", "http://localhost:8080", domain.ModeWhiteBox, domain.WithAllowlisted())
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	targetID, err := s.AddTarget(ctx, tg)
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}

	mustSLO := func(metric string, threshold float64, unit, comparator string, atRPS float64) domain.SLO {
		slo, err := domain.NewSLO(metric, threshold, unit, comparator, atRPS)
		if err != nil {
			t.Fatalf("NewSLO(%s): %v", metric, err)
		}
		return slo
	}

	if _, err := s.SetSLO(ctx, targetID, mustSLO("p99", 150, "ms", "<=", 200)); err != nil {
		t.Fatalf("SetSLO(p99): %v", err)
	}
	if _, err := s.SetSLO(ctx, targetID, mustSLO("error_rate", 0.1, "%", "<", 200)); err != nil {
		t.Fatalf("SetSLO(error_rate): %v", err)
	}

	// Re-declaring p99 is a set, not an append: threshold updates in place.
	if _, err := s.SetSLO(ctx, targetID, mustSLO("p99", 120, "ms", "<=", 200)); err != nil {
		t.Fatalf("SetSLO(p99 again): %v", err)
	}

	got, err := s.SLOsForTarget(ctx, targetID)
	if err != nil {
		t.Fatalf("SLOsForTarget: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d SLOs, want 2 (p99 upserted, not appended)", len(got))
	}

	byMetric := map[domain.MetricKind]domain.SLO{}
	for _, slo := range got {
		byMetric[slo.Metric] = slo
	}
	if p99 := byMetric[domain.MetricP99]; p99.Threshold != 120 {
		t.Errorf("p99 threshold = %v, want 120 (the re-set value)", p99.Threshold)
	}
	if er, ok := byMetric[domain.MetricErrorRate]; !ok || er.Unit != "%" || er.Comparator != domain.CmpLT {
		t.Errorf("error_rate SLO = %+v, want %%/< round-trip", er)
	}
	if p99 := byMetric[domain.MetricP99]; p99.AtRPS != 200 {
		t.Errorf("p99 at_rps = %v, want 200", p99.AtRPS)
	}
}
