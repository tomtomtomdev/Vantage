package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewConstantProfile_valid(t *testing.T) {
	p, err := NewConstantProfile(100, 60*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Kind != ProfileConstant || p.RPS != 100 || p.Duration != 60*time.Second || p.Warmup != 10*time.Second {
		t.Fatalf("fields not set: %+v", p)
	}
	if p.MeasuredWindow() != 50*time.Second {
		t.Errorf("MeasuredWindow = %s, want 50s", p.MeasuredWindow())
	}
}

func TestNewConstantProfile_validation(t *testing.T) {
	tests := []struct {
		name    string
		rps     int
		dur     time.Duration
		warmup  time.Duration
		wantErr error
	}{
		{"zero rps", 0, time.Second, 0, ErrProfileInvalid},
		{"negative rps", -1, time.Second, 0, ErrProfileInvalid},
		{"zero duration", 100, 0, 0, ErrProfileInvalid},
		{"negative warmup", 100, time.Second, -time.Second, ErrProfileInvalid},
		{"warmup >= duration", 100, 10 * time.Second, 10 * time.Second, ErrProfileInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewConstantProfile(tt.rps, tt.dur, tt.warmup); !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}

func TestParseProfile(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    LoadProfile
		wantErr bool
	}{
		{
			name: "full constant",
			in:   "constant:rps=100,dur=60s,warmup=10s",
			want: LoadProfile{Kind: ProfileConstant, RPS: 100, Duration: 60 * time.Second, Warmup: 10 * time.Second},
		},
		{
			name: "warmup defaults to zero",
			in:   "constant:rps=50,dur=30s",
			want: LoadProfile{Kind: ProfileConstant, RPS: 50, Duration: 30 * time.Second, Warmup: 0},
		},
		{"unknown kind", "ramp:from=1,to=10", LoadProfile{}, true},
		{"missing rps", "constant:dur=10s", LoadProfile{}, true},
		{"missing dur", "constant:rps=10", LoadProfile{}, true},
		{"bad dur", "constant:rps=10,dur=nope", LoadProfile{}, true},
		{"bad rps", "constant:rps=x,dur=10s", LoadProfile{}, true},
		{"no colon", "constant", LoadProfile{}, true},
		{"warmup ge dur", "constant:rps=10,dur=10s,warmup=10s", LoadProfile{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProfile(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
