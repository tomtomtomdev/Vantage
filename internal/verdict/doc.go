// Package verdict is the crown jewel: the significance math. Judge renders a
// PASS/FAIL against an SLO; Compare renders a baseline-vs-candidate delta with a
// bootstrap CI and a significant | within-noise | confounded judgment
// (SPEC §8, PROGRESS decisions log).
//
// Pure by mandate (CLAUDE.md §3) so it is tested exhaustively with fixtures in
// microseconds and never needs a database to prove a statistics bug is fixed.
// The bootstrap uses cluster/hierarchical resampling across reps; significant
// iff the CI excludes 0. Explicitly NOT σ-bands (invalid on tail percentiles).
package verdict
