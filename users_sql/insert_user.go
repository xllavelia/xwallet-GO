package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertUser(ctx context.Context, pool *pgxpool.Pool, playerID string, username string, passwordHash string, isAdmin bool) error {
	sqlQuery := `
	INSERT INTO users(player_id, username, password_hash, is_admin)
	VALUES ($1, $2, $3, $4);
	`
	_, err := pool.Exec(ctx, sqlQuery, playerID, username, passwordHash, isAdmin)

	return err
}
