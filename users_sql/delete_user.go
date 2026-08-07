package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DeleteUserByPlayerID(ctx context.Context, pool *pgxpool.Pool, targetPlayerID string) error {
	_, err := pool.Exec(ctx, `DELETE users WHERE user_id = 1$;`, targetPlayerID)
	return err
}
