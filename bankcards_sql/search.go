package bankcards_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CardSearchResult struct {
	CardNumber string
	Username   string
	PlayerID   string
	Tier       string
}

func SearchCardsByPrefix(ctx context.Context, pool *pgxpool.Pool, prefix string, excludeUserID int) ([]CardSearchResult, error) {
	rows, err := pool.Query(ctx, `
		SELECT bc.card_number, u.username, u.player_id, bc.tier
		FROM bank_cards bc
		JOIN users u ON u.id = bc.user_id
		WHERE bc.card_number LIKE $1 || '%' AND bc.user_id != $2
		ORDER BY u.username ASC
		LIMIT 9;
	`, prefix, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []CardSearchResult{}
	for rows.Next() {
		var r CardSearchResult
		if err := rows.Scan(&r.CardNumber, &r.Username, &r.PlayerID, &r.Tier); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
