package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateIdempotencyKey(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS client_request_id VARCHAR(64);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_positions_client_request ON positions (user_id, client_request_id) WHERE client_request_id IS NOT NULL;
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
