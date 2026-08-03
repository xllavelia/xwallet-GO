package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CountFriendsInvited(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM referral_links WHERE referrer_user_id = $1;`, userID).Scan(&count)

	return count, err
}

func CountActiveTraders(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT rl.referred_user_id)
		FROM referral_links rl
		WHERE rl.referrer_user_id = $1
		  AND EXISTS (SELECT 1 FROM positions p WHERE p.user_id = rl.referred_user_id);
	`, userID).Scan(&count)

	return count, err
}
