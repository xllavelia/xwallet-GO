package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateTimeTradeSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS trade_mode VARCHAR(8) NOT NULL DEFAULT 'standard';
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP;
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS payout_multiplier NUMERIC(5,2);
	CREATE INDEX IF NOT EXISTS idx_positions_time_settle ON positions (trade_mode, status, expires_at);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
