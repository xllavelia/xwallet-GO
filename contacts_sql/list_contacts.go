package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ListContacts(ctx context.Context, pool *pgxpool.Pool, userID int) ([]ContactItem, error) {
	sqlQuery := `
	SELECT u.player_id, u.username
	FROM contacts c
	JOIN users u ON u.id = c.contact_user_id
	WHERE c.user_id = $1
	ORDER BY c.created_at DESC
	LIMIT 9;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ContactItem
	for rows.Next() {
		var c ContactItem
		if err := rows.Scan(&c.PlayerID, &c.Username); err != nil {
			return nil, err
		}
		result = append(result, c)
	}

	return result, rows.Err()
}
