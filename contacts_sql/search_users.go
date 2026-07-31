package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SearchUsers(ctx context.Context, pool *pgxpool.Pool, query string, requestingUserID int) ([]UserSearchResult, error) {
	sqlQuery := `
	SELECT u.player_id, u.username, (c.id IS NOT NULL) AS is_contact
	FROM users u
	LEFT JOIN contacts c ON c.user_id = $2 AND c.contact_user_id = u.id
	WHERE (u.player_id LIKE '%' || $1 || '%' OR u.username ILIKE '%' || $1 || '%')
	  AND u.id != $2
	ORDER BY (CASE WHEN u.player_id = $1 THEN 0 ELSE 1 END), u.username ASC
	LIMIT 9;
	`
	rows, err := pool.Query(ctx, sqlQuery, query, requestingUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UserSearchResult
	for rows.Next() {
		var r UserSearchResult
		if err := rows.Scan(&r.PlayerID, &r.Username, &r.IsContact); err != nil {
			return nil, err
		}
		result = append(result, r)
	}

	return result, rows.Err()
}
