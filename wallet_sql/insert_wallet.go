package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const insertWalletQuery = `
	INSERT INTO wallets (user_id)
	VALUES ($1)
	ON CONFLICT (user_id) DO NOTHING;
	`

func InsertWallet(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, insertWalletQuery, userID)
	return err
}

func InsertWalletTx(ctx context.Context, tx pgx.Tx, userID int) error {
	_, err := tx.Exec(ctx, insertWalletQuery, userID)
	return err
}
