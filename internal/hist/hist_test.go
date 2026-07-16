package hist

import (
	"testing"
	"time"
)

func TestRecordAndPercentiles(t *testing.T) {
	h := New()
	// 1000 samples at 10ms, one outlier at 500ms — the tail must show it.
	for i := 0; i < 1000; i++ {
		if err := h.RecordDuration(10 * time.Millisecond); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := h.RecordDuration(500 * time.Millisecond); err != nil {
		t.Fatalf("record outlier: %v", err)
	}

	if got := h.TotalCount(); got != 1001 {
		t.Fatalf("TotalCount = %d, want 1001", got)
	}
	// p50 sits in the body (~10ms), the max reflects the outlier (~500ms).
	if p50 := h.PercentileMillis(50); p50 < 9 || p50 > 11 {
		t.Errorf("p50 = %.2fms, want ~10ms", p50)
	}
	if max := h.MaxMillis(); max < 490 || max > 510 {
		t.Errorf("max = %.2fms, want ~500ms", max)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	h := New()
	for i := 0; i < 500; i++ {
		_ = h.RecordDuration(time.Duration(i) * time.Millisecond)
	}

	blob, err := h.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("encoded blob is empty")
	}

	got, err := Decode(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TotalCount() != h.TotalCount() {
		t.Errorf("count after round-trip = %d, want %d", got.TotalCount(), h.TotalCount())
	}
	if got.PercentileMillis(99) != h.PercentileMillis(99) {
		t.Errorf("p99 after round-trip = %.3f, want %.3f", got.PercentileMillis(99), h.PercentileMillis(99))
	}
}

func TestMerge(t *testing.T) {
	a, b := New(), New()
	for i := 0; i < 100; i++ {
		_ = a.RecordDuration(10 * time.Millisecond)
		_ = b.RecordDuration(20 * time.Millisecond)
	}
	a.Merge(b)
	if got := a.TotalCount(); got != 200 {
		t.Fatalf("merged count = %d, want 200", got)
	}
}
