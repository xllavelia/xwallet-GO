package savings_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientWalletBalance = errors.New("insufficient wallet balance")

func Deposit(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) error {
	if err := ensureRow(ctx, pool, userID); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var newWalletBalance float64
	err = tx.QueryRow(ctx, `
		UPDATE wallets SET balance = balance - $1, updated_at = now()
		WHERE user_id = $2 AND balance >= $1
		RETURNING balance;
	`, amount, userID).Scan(&newWalletBalance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientWalletBalance
		}
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE savings_accounts SET balance = balance + $1 WHERE user_id = $2;`, amount, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO savings_history (user_id, entry_type, amount) VALUES ($1, 'deposit', $2);`, userID, amount); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
