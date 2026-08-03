package card_history_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertEntry(ctx context.Context, tx pgx.Tx, userID int, operationType string, fromAsset string, toAsset string, fromAmount float64, toAmount float64, price float64) error {
	sqlQuery := `
	INSERT INTO card_history (user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price)
	VALUES ($1, $2, $3, $4, $5, $6, $7);
	`
	_, err := tx.Exec(ctx, sqlQuery, userID, operationType, fromAsset, toAsset, fromAmount, toAmount, price)

	return err
}

func InsertEntryPool(ctx context.Context, pool *pgxpool.Pool, userID int, operationType string, fromAsset string, toAsset string, fromAmount float64, toAmount float64, price float64) error {
	sqlQuery := `
	INSERT INTO card_history (user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price)
	VALUES ($1, $2, $3, $4, $5, $6, $7);
	`
	_, err := pool.Exec(ctx, sqlQuery, userID, operationType, fromAsset, toAsset, fromAmount, toAmount, price)

	return err
}
