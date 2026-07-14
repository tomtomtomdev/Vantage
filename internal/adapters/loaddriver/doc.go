// Package loaddriver implements ports.LoadDriver: the open-model Go load driver
// (F1, the main event — SPEC §5, measuring-cleanly.md).
//
// Non-negotiable measurement integrity (CLAUDE.md §4, perf-measurement-rigor):
// arrival schedule is INDEPENDENT of completion (the coordinated-omission fix —
// do not gate the next arrival on the previous response); latency is measured
// from INTENDED dispatch time; a steady-state warmup window is discarded;
// percentiles are over successful requests only. The pool is bounded; every
// goroutine has an owner and a ctx-driven exit; the kill switch is ctx cancel.
// Tests run with -race and go.uber.org/goleak.
package loaddriver
