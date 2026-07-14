package domain

import (
	"errors"
	"testing"
)

func TestNewTarget_valid(t *testing.T) {
	got, err := NewTarget("sluice-ohlc", "https://sluice.internal/positions", ModeWhiteBox)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "sluice-ohlc" || got.BaseURL != "https://sluice.internal/positions" || got.Mode != ModeWhiteBox {
		t.Fatalf("fields not set: %+v", got)
	}
	// Safe by default: a fresh Target neither mutates nor is cleared to receive load (SPEC §9).
	if got.Mutating {
		t.Errorf("Mutating should default to false (safe)")
	}
	if got.Allowlisted {
		t.Errorf("Allowlisted should default to false — no allowlist entry, no load")
	}
}

func TestNewTarget_options(t *testing.T) {
	got, err := NewTarget("orders", "https://sandbox.internal/orders", ModeBlackBox,
		WithAllowlisted(), WithMutating())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Allowlisted || !got.Mutating {
		t.Fatalf("options not applied: %+v", got)
	}
}

func TestNewTarget_validation(t *testing.T) {
	tests := []struct {
		name    string
		tname   string
		baseURL string
		mode    Mode
		wantErr error
	}{
		{"empty name", "", "https://x.io", ModeWhiteBox, ErrTargetNameRequired},
		{"whitespace name", "   ", "https://x.io", ModeWhiteBox, ErrTargetNameRequired},
		{"empty url", "t", "", ModeWhiteBox, ErrTargetBaseURL},
		{"url no scheme", "t", "sluice.internal/x", ModeWhiteBox, ErrTargetBaseURL},
		{"url no host", "t", "https://", ModeWhiteBox, ErrTargetBaseURL},
		{"url not http", "t", "ftp://x.io", ModeWhiteBox, ErrTargetBaseURL},
		{"invalid mode", "t", "https://x.io", Mode("gray-box"), ErrTargetModeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTarget(tt.tname, tt.baseURL, tt.mode)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}

func TestMode_Valid(t *testing.T) {
	for _, m := range []Mode{ModeBlackBox, ModeWhiteBox} {
		if !m.Valid() {
			t.Errorf("%q should be valid", m)
		}
	}
	if Mode("").Valid() || Mode("gray-box").Valid() {
		t.Errorf("unknown modes must be invalid")
	}
}
