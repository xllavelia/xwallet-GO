package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetInternalIDByPlayerID(ctx context.Context, pool *pgxpool.Pool, playerID string) (int, error) {
	sqlQuery := `SELECT id FROM users WHERE player_id = $1;`

	var id int
	err := pool.QueryRow(ctx, sqlQuery, playerID).Scan(&id)

	return id, err
}
