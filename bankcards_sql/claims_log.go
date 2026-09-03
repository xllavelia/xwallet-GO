package bankcards_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func LogVoucherClaim(ctx context.Context, q Queryer, userID int, voucherType string, amount float64, source string) error {
	_, err := q.Exec(ctx, `INSERT INTO voucher_claims_log (user_id, voucher_type, amount, source) VALUES ($1, $2, $3, $4);`, userID, voucherType, amount, source)
	return err
}

func SumVoucherClaimsThisMonth(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	var total float64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM voucher_claims_log
		WHERE user_id = $1 AND voucher_type = 'usdt_credit'
		  AND claimed_at >= date_trunc('month', now());
	`, userID).Scan(&total)
	return total, err
}
