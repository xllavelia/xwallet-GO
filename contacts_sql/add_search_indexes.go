package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AddSearchIndexes(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE INDEX IF NOT EXISTS idx_users_player_id_trgm ON users USING gin (player_id gin_trgm_ops);
	CREATE INDEX IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}