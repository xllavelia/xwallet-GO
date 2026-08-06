package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

var timedVoucherTypes = []string{"fee_discount", "xp_boost", "fee_boost"}

func ActivateTimedVoucher(ctx context.Context, pool *pgxpool.Pool, id int, userID int) (string, bool, error) {
	for _, vt := range timedVoucherTypes {
		tag, err := pool.Exec(ctx, `
			UPDATE user_vouchers SET status = 'active', activated_at = now()
			WHERE id = $1 AND user_id = $2 AND voucher_type = $3 AND status = 'inactive';
		`, id, userID, vt)
		if err != nil {
			return "", false, err
		}
		if tag.RowsAffected() > 0 {
			return vt, true, nil
		}
	}
	return "", false, nil
}
