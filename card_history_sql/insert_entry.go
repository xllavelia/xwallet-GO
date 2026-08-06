package card_history_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertEntry(ctx context.Context, tx pgx.Tx, userID int, operationType string, fromAsset string, toAsset string, fromAmount float64, toAmount float64, price float64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO card_history (user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price)
		VALUES ($1, $2, $3, $4, $5, $6, $7);
	`, userID, operationType, fromAsset, toAsset, fromAmount, toAmount, price)
	return err
}

func InsertEntryPool(ctx context.Context, pool *pgxpool.Pool, userID int, operationType string, fromAsset string, toAsset string, fromAmount float64, toAmount float64, price float64, xpAwarded int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO card_history (user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price, xp_awarded)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`, userID, operationType, fromAsset, toAsset, fromAmount, toAmount, price, xpAwarded)
	return err
}
