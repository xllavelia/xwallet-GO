package wallet_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficientBalanceTx = errors.New("insufficient balance")

func AdjustBalanceTx(ctx context.Context, tx pgx.Tx, userID int, delta float64) error {
	if delta >= 0 {
		_, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;`, delta, userID)
		return err
	}

	var newBalance float64
	err := tx.QueryRow(ctx, `
		UPDATE wallets SET balance = balance + $1, updated_at = now()
		WHERE user_id = $2 AND balance >= $3
		RETURNING balance;
	`, delta, userID, -delta).Scan(&newBalance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientBalanceTx
		}
		return err
	}

	return nil
}
