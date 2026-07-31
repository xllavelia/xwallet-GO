package transfer_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AddReferenceCodeColumn(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `ALTER TABLE transfers ADD COLUMN IF NOT EXISTS reference_code VARCHAR(24);`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
