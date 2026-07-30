package transfer_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetTransfersByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Transfer, error) {
	sqlQuery := `
	SELECT t.id, t.sender_id, t.recipient_id, su.username, ru.username, t.amount, t.status, t.created_at
	FROM transfers t
	JOIN users su ON su.id = t.sender_id
	JOIN users ru ON ru.id = t.recipient_id
	WHERE t.sender_id = $1 OR t.recipient_id = $1
	ORDER BY t.created_at DESC
	LIMIT 100;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Transfer
	for rows.Next() {
		var t Transfer
		err := rows.Scan(&t.ID, &t.SenderID, &t.RecipientID, &t.SenderName, &t.RecipientName, &t.Amount, &t.Status, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}

	return result, rows.Err()
}
