package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdatePasswordHash(ctx context.Context, pool *pgxpool.Pool, userID int, newHash string) error {
	_, err := pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2;`, newHash, userID)
	return err
}
