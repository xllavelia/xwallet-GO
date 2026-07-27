package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func PlayerIDExists(ctx context.Context, pool *pgxpool.Pool, playerID string) (bool, error) {
	sqlQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE player_id = $1);`

	var exists bool
	err := pool.QueryRow(ctx, sqlQuery, playerID).Scan(&exists)

	return exists, err
}
