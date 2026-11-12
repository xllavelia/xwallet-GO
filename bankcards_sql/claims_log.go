package bankcards_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SumVoucherClaimsThisMonth(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	var total float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM voucher_claims_log
		WHERE user_id = $1 AND voucher_type = 'usdt_credit'
		  AND claimed_at >= date_trunc('month', now());
	`, userID).Scan(&total)
	return total, err
}
