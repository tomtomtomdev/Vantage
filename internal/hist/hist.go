// Package hist wraps HdrHistogram/hdrhistogram-go behind a small, pure API so the
// rest of Plumber records latencies and (de)serializes histograms without taking a
// direct dependency on the library's shape. It is pure computation — no I/O — so
// the load driver uses it now and the verdict bootstrap (S3) resamples from the
// same encoded blobs later.
//
// Latencies are tracked internally in microseconds (sub-millisecond precision for
// fast endpoints) and reported in milliseconds to match SPEC §6's reps columns.
package hist

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	hdr "github.com/HdrHistogram/hdrhistogram-go"
)

// Bounds: 1µs lowest discernible, 300s highest trackable, 3 significant figures.
// 300s comfortably exceeds any request timeout we'd set; values above are clamped
// to the max bucket rather than erroring, which is the honest thing for an outlier.
const (
	lowestMicros  = 1
	highestMicros = 300 * 1_000_000
	sigFigures    = 3
)

// Histogram accumulates latency samples and answers percentile queries.
type Histogram struct {
	h *hdr.Histogram
}

// New returns an empty Histogram over the standard latency bounds.
func New() *Histogram {
	return &Histogram{h: hdr.New(lowestMicros, highestMicros, sigFigures)}
}

// RecordDuration records one latency sample. A value above the trackable ceiling
// is clamped to the ceiling (recorded, not dropped) so a pathological outlier
// still shows up in the tail.
func (h *Histogram) RecordDuration(d time.Duration) error {
	us := d.Microseconds()
	if us < lowestMicros {
		us = lowestMicros
	}
	if us > highestMicros {
		us = highestMicros
	}
	if err := h.h.RecordValue(us); err != nil {
		return fmt.Errorf("recording %s: %w", d, err)
	}
	return nil
}

// TotalCount is the number of samples recorded.
func (h *Histogram) TotalCount() int64 { return h.h.TotalCount() }

// PercentileMillis returns the value at the given percentile (0–100) in ms.
func (h *Histogram) PercentileMillis(p float64) float64 {
	return microsToMillis(h.h.ValueAtPercentile(p))
}

// MaxMillis returns the largest recorded value in ms.
func (h *Histogram) MaxMillis() float64 { return microsToMillis(h.h.Max()) }

// Merge folds another histogram into this one — used to pool per-rep histograms
// into a Run-level distribution (SPEC §6).
func (h *Histogram) Merge(o *Histogram) { h.h.Merge(o.h) }

// Encode serializes the histogram to a compact blob for reps.histogram (bytea).
func (h *Histogram) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(h.h.Export()); err != nil {
		return nil, fmt.Errorf("encoding histogram: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode reconstructs a histogram from a blob produced by Encode.
func Decode(b []byte) (*Histogram, error) {
	var snap hdr.Snapshot
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decoding histogram: %w", err)
	}
	return &Histogram{h: hdr.Import(&snap)}, nil
}

func microsToMillis(us int64) float64 { return float64(us) / 1000.0 }
