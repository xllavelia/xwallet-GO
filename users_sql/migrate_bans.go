package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateBanSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS banned BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS ban_reason TEXT NOT NULL DEFAULT 'unknown';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS device_id VARCHAR(64);`,
		`CREATE TABLE IF NOT EXISTS banned_devices (
			device_id VARCHAR(64) PRIMARY KEY,
			reason TEXT NOT NULL DEFAULT 'unknown',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key VARCHAR(64) PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
	}
	for _, s := range statements {
		if _, err := pool.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
