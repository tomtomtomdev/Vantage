// Package report implements ports.ReportRenderer: the audit artifact renderer
// (Markdown first, then HTML — SPEC §11, PLAN S7).
//
// Tested by golden-master (CLAUDE.md §5): render a fixture Run, compare to an
// approved snapshot; any diff is a reviewed behaviour change. Non-determinism
// (timestamps, RNG) is scrubbed before comparison. The web dashboard in the
// design handoff (SPEC §10) is a LATER presentation layer over the same app
// API — not part of the MVP.
package report
