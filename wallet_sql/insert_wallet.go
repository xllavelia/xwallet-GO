package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertWallet(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	sqlQuery := `
	INSERT INTO wallets (user_id)
	VALUES ($1)
	ON CONFLICT (user_id) DO NOTHING;
	`
	_, err := pool.Exec(ctx, sqlQuery, userID)

	return err
}
