package domain

// Result is the Run-level pooled view derived from a Run's repetitions (SPEC §3):
// the merged-histogram percentiles plus the co-equal error-rate axis and achieved
// throughput. It is a pure value of plain numbers so verdict.Judge is tested with
// hand-built fixtures in microseconds and never needs a histogram or a database
// (CLAUDE §3). The app derives it from the reps (app.Summarise); the domain only
// declares its shape and what an SLO means against it.
//
// ErrorRate is a fraction in [0,1]; an error_rate SLO is expressed in % and the
// SLO does the conversion (see slo.go), so this stays the single source unit.
type Result struct {
	P50Ms  float64
	P90Ms  float64
	P99Ms  float64
	P999Ms float64
	MaxMs  float64

	ErrorRate     float64 // fraction in [0,1] over successful vs total measured
	ThroughputRPS float64 // achieved arrival/completion rate
}
