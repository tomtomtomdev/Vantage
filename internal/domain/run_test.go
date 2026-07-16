package domain

import (
	"errors"
	"testing"
	"time"
)

func validProfile(t *testing.T) LoadProfile {
	t.Helper()
	p, err := NewConstantProfile(100, 10*time.Second, time.Second)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	return p
}

func TestRun_Validate(t *testing.T) {
	p := validProfile(t)
	reps := []RepResult{{Seq: 1}, {Seq: 2}}

	tests := []struct {
		name    string
		run     Run
		wantErr error
	}{
		{
			name: "valid",
			run:  Run{Profile: p, NReps: 2, Reps: reps},
		},
		{
			name:    "n_reps below one",
			run:     Run{Profile: p, NReps: 0, Reps: nil},
			wantErr: ErrRunInvalid,
		},
		{
			name:    "rep count mismatch",
			run:     Run{Profile: p, NReps: 3, Reps: reps},
			wantErr: ErrRunInvalid,
		},
		{
			name:    "profile unset",
			run:     Run{Profile: LoadProfile{}, NReps: 2, Reps: reps},
			wantErr: ErrRunInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}
