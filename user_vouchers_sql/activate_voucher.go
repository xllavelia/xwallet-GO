package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ActivateFeeVoucher(ctx context.Context, pool *pgxpool.Pool, id int, userID int) (bool, error) {
	sqlQuery := `
	UPDATE user_vouchers SET status = 'active', activated_at = now()
	WHERE id = $1 AND user_id = $2 AND voucher_type = 'fee_discount' AND status = 'inactive';
	`
	tag, err := pool.Exec(ctx, sqlQuery, id, userID)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() > 0, nil
}
