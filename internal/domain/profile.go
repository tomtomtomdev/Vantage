package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ProfileKind names a traffic shape (SPEC §3). S1 ships constant only; ramp,
// spike and soak arrive in later slices.
type ProfileKind string

const ProfileConstant ProfileKind = "constant"

// ErrProfileInvalid is the sentinel for a malformed or contradictory profile.
var ErrProfileInvalid = errors.New("profile: invalid")

// LoadProfile is how traffic is shaped over a repetition. Every profile carries a
// Warmup window whose samples are discarded so only steady-state is measured
// (SPEC §8; perf-measurement-rigor: warmup counted). It is a pure value.
type LoadProfile struct {
	Kind     ProfileKind
	RPS      int           // requested arrival rate (open-model; independent of completion)
	Duration time.Duration // total wall-clock length of one repetition
	Warmup   time.Duration // leading window discarded from the histogram and error tally
}

// NewConstantProfile validates and constructs a constant-rate profile.
func NewConstantProfile(rps int, duration, warmup time.Duration) (LoadProfile, error) {
	if rps <= 0 {
		return LoadProfile{}, fmt.Errorf("%w: rps must be positive, got %d", ErrProfileInvalid, rps)
	}
	if duration <= 0 {
		return LoadProfile{}, fmt.Errorf("%w: duration must be positive, got %s", ErrProfileInvalid, duration)
	}
	if warmup < 0 {
		return LoadProfile{}, fmt.Errorf("%w: warmup must not be negative, got %s", ErrProfileInvalid, warmup)
	}
	if warmup >= duration {
		return LoadProfile{}, fmt.Errorf("%w: warmup %s must be shorter than duration %s", ErrProfileInvalid, warmup, duration)
	}
	return LoadProfile{Kind: ProfileConstant, RPS: rps, Duration: duration, Warmup: warmup}, nil
}

// MeasuredWindow is the steady-state window after the warmup is discarded.
func (p LoadProfile) MeasuredWindow() time.Duration { return p.Duration - p.Warmup }

// String renders the profile in the CLI's own flag syntax, so a run's profile is
// copy-pasteable back into `vantage run --profile`.
func (p LoadProfile) String() string {
	return fmt.Sprintf("%s:rps=%d,dur=%s,warmup=%s", p.Kind, p.RPS, p.Duration, p.Warmup)
}

// ParseProfile parses the CLI flag syntax "constant:rps=100,dur=60s,warmup=10s".
// warmup is optional and defaults to zero. It is pure so it is unit-tested without
// the CLI.
func ParseProfile(s string) (LoadProfile, error) {
	kind, rest, ok := strings.Cut(s, ":")
	if !ok {
		return LoadProfile{}, fmt.Errorf("%w: expected \"kind:key=val,...\", got %q", ErrProfileInvalid, s)
	}
	if ProfileKind(kind) != ProfileConstant {
		return LoadProfile{}, fmt.Errorf("%w: unknown kind %q (only %q is supported)", ErrProfileInvalid, kind, ProfileConstant)
	}

	fields := map[string]string{}
	for _, pair := range strings.Split(rest, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return LoadProfile{}, fmt.Errorf("%w: bad field %q", ErrProfileInvalid, pair)
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	rpsStr, ok := fields["rps"]
	if !ok {
		return LoadProfile{}, fmt.Errorf("%w: missing rps", ErrProfileInvalid)
	}
	rps, err := strconv.Atoi(rpsStr)
	if err != nil {
		return LoadProfile{}, fmt.Errorf("%w: rps %q: %v", ErrProfileInvalid, rpsStr, err)
	}

	durStr, ok := fields["dur"]
	if !ok {
		return LoadProfile{}, fmt.Errorf("%w: missing dur", ErrProfileInvalid)
	}
	duration, err := time.ParseDuration(durStr)
	if err != nil {
		return LoadProfile{}, fmt.Errorf("%w: dur %q: %v", ErrProfileInvalid, durStr, err)
	}

	var warmup time.Duration
	if warmupStr, ok := fields["warmup"]; ok {
		warmup, err = time.ParseDuration(warmupStr)
		if err != nil {
			return LoadProfile{}, fmt.Errorf("%w: warmup %q: %v", ErrProfileInvalid, warmupStr, err)
		}
	}

	return NewConstantProfile(rps, duration, warmup)
}
