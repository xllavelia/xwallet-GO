package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigratePlayerIDToVarchar(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE users
	ALTER COLUMN player_id TYPE VARCHAR(6)
	USING TRIM(player_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
