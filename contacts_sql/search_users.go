package contacts_sql

import (
	"context"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SearchUsers(ctx context.Context, pool *pgxpool.Pool, rawQuery string, requestingUserID int) ([]UserSearchResult, error) {
	query := strings.TrimSpace(rawQuery)

	sqlQuery := `
	SELECT
		TRIM(u.player_id)::text AS player_id,
		u.username::text        AS username,
		(c.id IS NOT NULL)      AS is_contact
	FROM users u
	LEFT JOIN contacts c
		ON c.user_id = $2
		AND c.contact_user_id = u.id
	WHERE u.id != $2
		AND (
			TRIM(u.player_id)::text ILIKE '%' || $1::text || '%'
			OR u.username::text ILIKE '%' || $1::text || '%'
		)
	ORDER BY
		(CASE WHEN TRIM(u.player_id)::text = $1::text THEN 0 ELSE 1 END),
		u.username ASC
	LIMIT 9;
	`

	rows, err := pool.Query(ctx, sqlQuery, query, requestingUserID)
	if err != nil {
		log.Println("SearchUsers query error:", err, "| query=", query)
		return nil, err
	}
	defer rows.Close()

	var result []UserSearchResult
	for rows.Next() {
		var r UserSearchResult
		if err := rows.Scan(&r.PlayerID, &r.Username, &r.IsContact); err != nil {
			log.Println("SearchUsers scan error:", err)
			return nil, err
		}
		result = append(result, r)
	}

	if err := rows.Err(); err != nil {
		log.Println("SearchUsers rows error:", err)
		return nil, err
	}

	log.Println("SearchUsers: query=", query, "| found=", len(result), "rows")

	return result, nil
}
