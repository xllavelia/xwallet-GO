package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func FixDefaultBalance(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `ALTER TABLE wallets ALTER COLUMN balance SET DEFAULT 0;`)
	return err
}
