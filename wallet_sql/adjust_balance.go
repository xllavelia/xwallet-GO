package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AdjustBalance(ctx context.Context, pool *pgxpool.Pool, userID int, delta float64) error {
	sqlQuery := `
	UPDATE wallets
	SET balance = GREATEST(0, balance + $2), updated_at = now()
	WHERE user_id = $1;
	`
	_, err := pool.Exec(ctx, sqlQuery, userID, delta)

	return err
}

func GetBalanceByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	sqlQuery := `SELECT balance FROM wallets WHERE user_id = $1;`

	var balance float64
	err := pool.QueryRow(ctx, sqlQuery, userID).Scan(&balance)

	return balance, err
}
