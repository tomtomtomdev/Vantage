package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The command tree and its arg/flag validation are wired here; test the parts
// that fail BEFORE any database connection, so the suite needs no container.
func TestCommandTree(t *testing.T) {
	root := newRootCmd()

	has := func(parent string) map[string]bool {
		cmd := root
		if parent != "" {
			for _, c := range root.Commands() {
				if c.Name() == parent {
					cmd = c
				}
			}
		}
		names := map[string]bool{}
		for _, c := range cmd.Commands() {
			names[c.Name()] = true
		}
		return names
	}

	top := has("")
	if !top["migrate"] || !top["target"] || !top["run"] || !top["slo"] || !top["baseline"] || !top["compare"] {
		t.Fatalf("root subcommands = %v, want migrate + target + run + slo + baseline + compare", top)
	}
	sub := has("target")
	if !sub["add"] || !sub["list"] {
		t.Fatalf("target subcommands = %v, want add + list", sub)
	}
	slo := has("slo")
	if !slo["set"] || !slo["list"] {
		t.Fatalf("slo subcommands = %v, want set + list", slo)
	}
	base := has("baseline")
	if !base["set"] {
		t.Fatalf("baseline subcommands = %v, want set", base)
	}
}

func TestTargetAddValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing name", []string{"target", "add", "--url", "http://x"}, "arg"},
		{"missing required url", []string{"target", "add", "sluice"}, "url"},
		{"run missing target name", []string{"run", "--profile", "constant:rps=1,dur=1s"}, "arg"},
		{"run missing profile", []string{"run", "sluice"}, "profile"},
		// A malformed profile is rejected before any DB connection (domain-pure parse).
		{"run bad profile", []string{"run", "sluice", "--profile", "ramp:x=1"}, "profile"},
		{"slo set missing target name", []string{"slo", "set", "--metric", "p99", "--threshold", "150", "--unit", "ms", "--comparator", "<="}, "arg"},
		{"slo set missing required metric", []string{"slo", "set", "sluice", "--threshold", "150", "--unit", "ms", "--comparator", "<="}, "metric"},
		{"slo list missing target name", []string{"slo", "list"}, "arg"},
		{"baseline set missing run id", []string{"baseline", "set"}, "arg"},
		// A non-numeric run id is rejected before any DB connection.
		{"baseline set bad run id", []string{"baseline", "set", "abc"}, "run id"},
		{"compare missing run id", []string{"compare"}, "arg"},
		{"compare bad run id", []string{"compare", "notanum"}, "run id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(tt.args)

			err := root.ExecuteContext(context.Background())
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got nil", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
