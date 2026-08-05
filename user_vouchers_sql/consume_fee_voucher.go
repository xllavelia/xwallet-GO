package user_vouchers_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type ActiveFeeVoucher struct {
	ID          int
	LimitAmount float64
	UsedAmount  float64
}

var ErrNoActiveFeeVoucher = errors.New("no active fee voucher")

func FindActiveFeeVoucherForUpdate(ctx context.Context, tx pgx.Tx, userID int) (ActiveFeeVoucher, error) {
	var v ActiveFeeVoucher
	err := tx.QueryRow(ctx, `
		SELECT id, limit_amount, used_amount
		FROM user_vouchers
		WHERE user_id = $1
		  AND voucher_type = 'fee_discount'
		  AND status = 'active'
		  AND used_amount < limit_amount
		  AND activated_at + (duration_seconds || ' seconds')::interval > now()
		ORDER BY activated_at ASC
		LIMIT 1
		FOR UPDATE;
	`, userID).Scan(&v.ID, &v.LimitAmount, &v.UsedAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActiveFeeVoucher{}, ErrNoActiveFeeVoucher
		}
		return ActiveFeeVoucher{}, err
	}

	return v, nil
}

func ConsumeVoucherAmount(ctx context.Context, tx pgx.Tx, id int, amount float64) error {
	_, err := tx.Exec(ctx, `UPDATE user_vouchers SET used_amount = used_amount + $1 WHERE id = $2;`, amount, id)

	return err
}
