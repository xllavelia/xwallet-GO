package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DeleteFeeVoucher(ctx context.Context, pool *pgxpool.Pool, id int, userID int) (bool, error) {
	sqlQuery := `
	DELETE FROM user_vouchers
	WHERE id = $1 AND user_id = $2
	  AND voucher_type IN ('fee_discount', 'xp_boost', 'fee_boost')
	  AND (
	    status = 'inactive'
	    OR (status = 'active' AND activated_at + (duration_seconds || ' seconds')::interval < now())
	  );
	`
	tag, err := pool.Exec(ctx, sqlQuery, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func DeleteAllByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, `DELETE FROM user_vouchers WHERE user_id = $1;`, userID)
	return err
}
