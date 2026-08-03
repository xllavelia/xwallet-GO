package card_history_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetHistoryByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]CardHistoryEntry, error) {
	sqlQuery := `
	SELECT id, user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price, created_at
	FROM card_history
	WHERE user_id = $1
	ORDER BY created_at DESC
	LIMIT 100;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []CardHistoryEntry{}
	for rows.Next() {
		var e CardHistoryEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.OperationType, &e.FromAsset, &e.ToAsset, &e.FromAmount, &e.ToAmount, &e.Price, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}

	return result, rows.Err()
}

func GetEntryByID(ctx context.Context, pool *pgxpool.Pool, id int, userID int) (CardHistoryEntry, error) {
	sqlQuery := `
	SELECT id, user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price, created_at
	FROM card_history
	WHERE id = $1 AND user_id = $2;
	`
	var e CardHistoryEntry
	err := pool.QueryRow(ctx, sqlQuery, id, userID).Scan(&e.ID, &e.UserID, &e.OperationType, &e.FromAsset, &e.ToAsset, &e.FromAmount, &e.ToAmount, &e.Price, &e.CreatedAt)

	return e, err
}
