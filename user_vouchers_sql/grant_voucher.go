package user_vouchers_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSlotsFull = errors.New("all voucher slots are full")

func CountByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)

	return count, err
}

func GrantFeeDiscountVoucher(ctx context.Context, pool *pgxpool.Pool, userID int, limitAmount float64, durationSeconds int, source string) error {
	count, err := CountByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if count >= 5 {
		return ErrSlotsFull
	}

	sqlQuery := `
	INSERT INTO user_vouchers (user_id, voucher_type, status, limit_amount, duration_seconds, source)
	VALUES ($1, 'fee_discount', 'inactive', $2, $3, $4);
	`
	_, err = pool.Exec(ctx, sqlQuery, userID, limitAmount, durationSeconds, source)

	return err
}

func GrantCreditVoucher(ctx context.Context, pool *pgxpool.Pool, userID int, voucherType string, creditAmount float64, source string) error {
	if voucherType != "usdt_credit" && voucherType != "lavx_credit" {
		return errors.New("invalid credit voucher type")
	}

	count, err := CountByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if count >= 5 {
		return ErrSlotsFull
	}

	sqlQuery := `
	INSERT INTO user_vouchers (user_id, voucher_type, status, credit_amount, source)
	VALUES ($1, $2, 'inactive', $3, $4);
	`
	_, err = pool.Exec(ctx, sqlQuery, userID, voucherType, creditAmount, source)

	return err
}
