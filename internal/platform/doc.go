// Package platform holds cross-cutting mechanics: config (explicit struct,
// loaded once at the composition root, injected down), secrets, the migrations
// runner, and the Postgres connection (pgxpool).
//
// Secrets GATE (SPEC review #5, CLAUDE.md §7): targets.auth is encrypted at
// rest (secretbox/age, key from env/secrets-manager) or stored as a reference —
// NEVER a plaintext bearer token on disk. This gate blocks S1 touching any
// non-local target.
package platform
