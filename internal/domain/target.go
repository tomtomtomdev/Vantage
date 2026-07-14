package domain

import (
	"errors"
	"fmt"
	"net/url" // pure URL parsing, no I/O — distinct from the forbidden net/http (arch-test)
	"strings"
)

// Mode is how Plumber observes a Target: from the wire (black-box) or with
// server-side instrumentation for tail attribution (white-box). SPEC §5/§8.
type Mode string

const (
	ModeBlackBox Mode = "black-box"
	ModeWhiteBox Mode = "white-box"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeBlackBox, ModeWhiteBox:
		return true
	default:
		return false
	}
}

// Target is a system Plumber is authorized to measure. It is a pure value; the
// blast-radius flags are enforced at the driver's entry path (SPEC §9, S1), but
// they live on the Target so that enforcement has data to act on.
//
// The zero value is the SAFE state: a Target is neither Mutating nor
// Allowlisted until deliberately opted in — "no allowlist entry, no load".
type Target struct {
	Name        string
	BaseURL     string
	Mode        Mode
	Mutating    bool // endpoint changes state; refused by default (SPEC §9)
	Allowlisted bool // cleared to receive load; without it, hard stop (SPEC §9)
}

// Domain validation errors — typed sentinels, inspected with errors.Is.
var (
	ErrTargetNameRequired = errors.New("target: name required")
	ErrTargetBaseURL      = errors.New("target: base_url must be an absolute http(s) URL")
	ErrTargetModeInvalid  = errors.New("target: mode must be black-box or white-box")
)

// TargetOption opts a Target into a less-safe state at the call site, so the
// dangerous choices (mutating, allowlisted) are always explicit and greppable.
type TargetOption func(*Target)

// WithMutating marks the Target's endpoint as state-changing (SPEC §9): running
// it will later require an explicit override and disposable data.
func WithMutating() TargetOption { return func(t *Target) { t.Mutating = true } }

// WithAllowlisted marks the Target as cleared to receive load (SPEC §9).
func WithAllowlisted() TargetOption { return func(t *Target) { t.Allowlisted = true } }

// NewTarget validates and constructs a Target. Invalid states are refused at
// construction so every Target the system holds is already well-formed.
func NewTarget(name, baseURL string, mode Mode, opts ...TargetOption) (Target, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Target{}, ErrTargetNameRequired
	}
	if !absoluteHTTPURL(baseURL) {
		return Target{}, fmt.Errorf("%w: %q", ErrTargetBaseURL, baseURL)
	}
	if !mode.Valid() {
		return Target{}, fmt.Errorf("%w: %q", ErrTargetModeInvalid, mode)
	}

	t := Target{Name: name, BaseURL: baseURL, Mode: mode}
	for _, opt := range opts {
		opt(&t)
	}
	return t, nil
}

// absoluteHTTPURL reports whether s is an absolute http/https URL with a host.
func absoluteHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
