// Package migrations embeds the numbered SQL so the runner in
// internal/platform can apply it without a filesystem dependency at runtime.
// The SQL lives next to its embed directive (Go embed cannot reach outside the
// package directory); the runner takes an fs.FS so it stays testable.
package migrations

import "embed"

// FS holds the numbered *.sql migrations, applied in lexical order (CLAUDE §7).
//
//go:embed *.sql
var FS embed.FS
