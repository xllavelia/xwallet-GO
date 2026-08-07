package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateUsername(ctx context.Context, pool *pgxpool.Pool, userID int, newUsername string) error {
	_, err := pool.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2;`, newUsername, userID)
	return err
}
