package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const insertUserQuery = `
	INSERT INTO users(player_id, username, password_hash, is_admin)
	VALUES ($1, $2, $3, $4);
	`

func InsertUser(ctx context.Context, pool *pgxpool.Pool, playerID string, username string, passwordHash string, isAdmin bool) error {
	_, err := pool.Exec(ctx, insertUserQuery, playerID, username, passwordHash, isAdmin)
	return err
}

func InsertUserTx(ctx context.Context, tx pgx.Tx, playerID string, username string, passwordHash string, isAdmin bool) error {
	_, err := tx.Exec(ctx, insertUserQuery, playerID, username, passwordHash, isAdmin)
	return err
}
