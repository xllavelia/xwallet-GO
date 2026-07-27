package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateUsersTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS users(
		id SERIAL PRIMARY KEY,
		player_id CHAR(6) NOT NULL UNIQUE,
		username VARCHAR(32) NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin BOOLEAN NOT NULL DEFAULT false,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT player_id_digits_only CHECK (player_id ~ '^[0-9]{6}$')
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
