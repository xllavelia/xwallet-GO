package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateClaimsLogWidth(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `ALTER TABLE voucher_claims_log ALTER COLUMN source TYPE VARCHAR(32);`)
	return err
}
