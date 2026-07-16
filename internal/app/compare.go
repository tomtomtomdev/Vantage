package app

import (
	"context"
	"fmt"
	"strconv"

	"plumber/internal/domain"
	"plumber/internal/hist"
	"plumber/internal/ports"
	"plumber/internal/verdict"
)

// CompareService orchestrates the S3 verdict flavour: designate a baseline and
// compare a candidate Run against it (SPEC §7). It resolves the runs from the
// store, decodes their per-rep histograms, and hands clean data to the pure
// verdict.Compare — the app does the I/O, verdict does the significance math.
type CompareService struct {
	store ports.BaselineStore
}

// NewCompareService injects the store the use case reads runs and baselines from.
func NewCompareService(store ports.BaselineStore) *CompareService {
	return &CompareService{store: store}
}

// SetBaseline designates runID as the baseline for the target it was measured
// against, returning that target's id. A missing run surfaces domain.ErrRunNotFound.
func (s *CompareService) SetBaseline(ctx context.Context, runID int64) (int64, error) {
	_, targetID, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	if err := s.store.SetBaseline(ctx, targetID, runID); err != nil {
		return 0, fmt.Errorf("setting baseline: %w", err)
	}
	return targetID, nil
}

// CompareResult bundles the comparison with the identity of the two runs, so the
// CLI can render the "before-fix → after-index" report line without another lookup.
type CompareResult struct {
	Comparison       verdict.Comparison
	TargetID         int64
	BaselineRunID    int64
	CandidateRunID   int64
	BaselineLabel    string
	CandidateLabel   string
	BaselineVersion  string
	CandidateVersion string
}

// Compare judges a candidate Run against its target's current baseline. It refuses
// with domain.ErrNoBaseline when none is set — a delta needs a stored baseline
// (number-before-narrative, SPEC §8). The bootstrap options (seed, resamples) are
// passed through to verdict.Compare.
func (s *CompareService) Compare(ctx context.Context, candidateRunID int64, opts ...verdict.Option) (CompareResult, error) {
	candRun, targetID, err := s.store.GetRun(ctx, candidateRunID)
	if err != nil {
		return CompareResult{}, err
	}

	baseRunID, ok, err := s.store.Baseline(ctx, targetID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("loading baseline: %w", err)
	}
	if !ok {
		return CompareResult{}, fmt.Errorf("target %d: %w", targetID, domain.ErrNoBaseline)
	}

	baseRun, _, err := s.store.GetRun(ctx, baseRunID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("loading baseline run %d: %w", baseRunID, err)
	}

	baseIn, err := compareInput(baseRun, targetID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("baseline run %d: %w", baseRunID, err)
	}
	candIn, err := compareInput(candRun, targetID)
	if err != nil {
		return CompareResult{}, fmt.Errorf("candidate run %d: %w", candidateRunID, err)
	}

	return CompareResult{
		Comparison:       verdict.Compare(baseIn, candIn, opts...),
		TargetID:         targetID,
		BaselineRunID:    baseRunID,
		CandidateRunID:   candidateRunID,
		BaselineLabel:    baseRun.Label,
		CandidateLabel:   candRun.Label,
		BaselineVersion:  baseRun.VersionMarker,
		CandidateVersion: candRun.VersionMarker,
	}, nil
}

// compareInput decodes a Run's per-rep histograms and packages the guard metadata
// into a verdict.CompareInput. TargetName is the target id (both sides of a compare
// share it by construction), which is all the target-shape guard needs.
func compareInput(run domain.Run, targetID int64) (verdict.CompareInput, error) {
	hs := make([]*hist.Histogram, len(run.Reps))
	for i, rep := range run.Reps {
		h, err := hist.Decode(rep.Histogram)
		if err != nil {
			return verdict.CompareInput{}, fmt.Errorf("decoding rep %d histogram: %w", rep.Seq, err)
		}
		hs[i] = h
	}
	return verdict.CompareInput{
		Reps:       hs,
		Profile:    run.Profile,
		Env:        run.Env,
		TargetName: strconv.FormatInt(targetID, 10),
	}, nil
}
