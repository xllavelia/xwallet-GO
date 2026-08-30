package savings_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureRow(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO savings_accounts (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING;
	`, userID)
	return err
}

func GetAccount(ctx context.Context, pool *pgxpool.Pool, userID int) (Account, error) {
	if err := ensureRow(ctx, pool, userID); err != nil {
		return Account{}, err
	}

	var a Account
	err := pool.QueryRow(ctx, `
		SELECT user_id, balance, last_accrued_at FROM savings_accounts WHERE user_id = $1;
	`, userID).Scan(&a.UserID, &a.Balance, &a.LastAccruedAt)

	return a, err
}
