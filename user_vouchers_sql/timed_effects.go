package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetActiveXpBoostPercent(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	var percent float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(limit_amount), 0) FROM user_vouchers
		WHERE user_id = $1 AND voucher_type = 'xp_boost' AND status = 'active'
		  AND activated_at + (duration_seconds || ' seconds')::interval > now();
	`, userID).Scan(&percent)
	return percent, err
}

func GetActiveFeeBoostPoints(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	var points float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(limit_amount), 0) FROM user_vouchers
		WHERE user_id = $1 AND voucher_type = 'fee_boost' AND status = 'active'
		  AND activated_at + (duration_seconds || ' seconds')::interval > now();
	`, userID).Scan(&points)
	return points, err
}
