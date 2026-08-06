package card_history_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetHistoryByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]CardHistoryEntry, error) {
	sqlQuery := `
	SELECT id, user_id, operation_type, from_asset, to_asset, from_amount, to_amount, price, created_at, xp_awarded
	FROM card_history WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CardHistoryEntry{}
	for rows.Next() {
		var e CardHistoryEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.OperationType, &e.FromAsset, &e.ToAsset, &e.FromAmount, &e.ToAmount, &e.Price, &e.CreatedAt, &e.XpAwarded); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
