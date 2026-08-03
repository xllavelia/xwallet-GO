package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AddLavxBalanceColumn(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `ALTER TABLE wallets ADD COLUMN IF NOT EXISTS lavx_balance NUMERIC(14, 2) NOT NULL DEFAULT 0;`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
