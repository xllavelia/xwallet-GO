package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnableTrgmExtension(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `CREATE EXTENSION IF NOT EXISTS pg_trgm;`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}