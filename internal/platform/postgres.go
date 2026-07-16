package platform

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgxpool sized per Config and verifies connectivity with a
// ping, so a bad DSN fails fast at the composition root rather than on the first
// query. The caller owns the returned pool and must Close it.
func Connect(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	pcfg.MaxConns = cfg.PoolSize

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("opening pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}
