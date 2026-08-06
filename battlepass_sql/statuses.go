package battlepass_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetStatuses(ctx context.Context, pool *pgxpool.Pool, userID int) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT status FROM user_statuses WHERE user_id = $1 ORDER BY granted_at ASC;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
