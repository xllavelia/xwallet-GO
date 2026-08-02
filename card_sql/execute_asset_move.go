package card_sql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientAssetBalance = errors.New("insufficient balance")

func ExecuteAssetMove(ctx context.Context, pool *pgxpool.Pool, userID int, fromAsset string, toAsset string, fromAmount float64, toAmount float64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if fromAsset == "USDT" {
		var newBalance float64
		err := tx.QueryRow(ctx, `
			UPDATE wallets SET balance = balance - $1, updated_at = now()
			WHERE user_id = $2 AND balance >= $1
			RETURNING balance;
		`, fromAmount, userID).Scan(&newBalance)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInsufficientAssetBalance
			}
			return err
		}
	} else {
		column, ok := columnByCoin[fromAsset]
		if !ok {
			return fmt.Errorf("unsupported coin: %s", fromAsset)
		}
		sqlQuery := fmt.Sprintf(`
			UPDATE crypto_cards SET %s = %s - $1
			WHERE user_id = $2 AND %s >= $1
			RETURNING %s;
		`, column, column, column, column)
		var newAmount float64
		err := tx.QueryRow(ctx, sqlQuery, fromAmount, userID).Scan(&newAmount)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInsufficientAssetBalance
			}
			return err
		}
	}

	if toAsset == "USDT" {
		_, err := tx.Exec(ctx, `
			UPDATE wallets SET balance = balance + $1, updated_at = now()
			WHERE user_id = $2;
		`, toAmount, userID)
		if err != nil {
			return err
		}
	} else {
		column, ok := columnByCoin[toAsset]
		if !ok {
			return fmt.Errorf("unsupported coin: %s", toAsset)
		}
		sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s + $1 WHERE user_id = $2;`, column, column)
		_, err := tx.Exec(ctx, sqlQuery, toAmount, userID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
