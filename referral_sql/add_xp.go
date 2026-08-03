package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AddRefXP(ctx context.Context, pool *pgxpool.Pool, userID int, amount int) error {
	_, err := pool.Exec(ctx, `UPDATE referrals SET ref_xp = ref_xp + $1 WHERE user_id = $2;`, amount, userID)

	return err
}
