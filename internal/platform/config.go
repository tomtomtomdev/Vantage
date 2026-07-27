package platform

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the explicit configuration struct, loaded once at the composition
// root and injected down (CLAUDE §7). No package reads env vars deep in its guts.
type Config struct {
	// DatabaseURL is the pgx connection string (postgres://…). Required.
	DatabaseURL string
	// PoolSize caps the pgxpool; recorded on runs so measurements are reproducible.
	PoolSize int32
}

// ErrConfig signals missing or invalid configuration.
var ErrConfig = fmt.Errorf("platform: invalid configuration")

// LoadConfig reads configuration from the environment. It is the ONLY place env
// vars are consulted; everything else receives a Config by injection.
func LoadConfig() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("VANTAGE_DATABASE_URL"),
		PoolSize:    8,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("%w: VANTAGE_DATABASE_URL is required", ErrConfig)
	}
	if v := os.Getenv("VANTAGE_POOL_SIZE"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("%w: VANTAGE_POOL_SIZE must be a positive integer, got %q", ErrConfig, v)
		}
		cfg.PoolSize = int32(n)
	}
	return cfg, nil
}
