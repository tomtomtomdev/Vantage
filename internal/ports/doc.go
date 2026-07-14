// Package ports declares the interfaces the domain/app need, defined by the
// CONSUMER, not the implementer (CLAUDE.md §3): LoadDriver, ExemplarTraceSource,
// PlanSource, ResultStore, ReportRenderer, Clock.
//
// Keep interfaces small (1–3 methods). Adapters in internal/adapters implement
// these and return concrete structs ("accept interfaces, return structs").
package ports
