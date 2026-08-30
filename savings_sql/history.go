package savings_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetHistory(ctx context.Context, pool *pgxpool.Pool, userID int) ([]HistoryEntry, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, entry_type, amount, created_at
		FROM savings_history WHERE user_id = $1
		ORDER BY created_at DESC LIMIT 100;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.EntryType, &e.Amount, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func GetTotalAccruedInterest(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	var total float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM savings_history
		WHERE user_id = $1 AND entry_type = 'interest';
	`, userID).Scan(&total)
	return total, err
}
