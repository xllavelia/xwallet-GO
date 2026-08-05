package user_vouchers_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSlotsFull = errors.New("all voucher slots are full")

func CountByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)
	return count, err
}

func countByUserIDTx(ctx context.Context, tx pgx.Tx, userID int) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)
	return count, err
}

func GrantFeeDiscountVoucher(ctx context.Context, pool *pgxpool.Pool, userID int, limitAmount float64, durationSeconds int, source string, maxSlots int) error {
	count, err := CountByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, limit_amount, duration_seconds, source)
		VALUES ($1, 'fee_discount', 'inactive', $2, $3, $4);
	`, userID, limitAmount, durationSeconds, source)
	return err
}

func GrantFeeDiscountVoucherTx(ctx context.Context, tx pgx.Tx, userID int, limitAmount float64, durationSeconds int, source string, maxSlots int) error {
	count, err := countByUserIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, limit_amount, duration_seconds, source)
		VALUES ($1, 'fee_discount', 'inactive', $2, $3, $4);
	`, userID, limitAmount, durationSeconds, source)
	return err
}

func GrantCreditVoucher(ctx context.Context, pool *pgxpool.Pool, userID int, voucherType string, creditAmount float64, source string, maxSlots int) error {
	if voucherType != "usdt_credit" && voucherType != "lavx_credit" && voucherType != "ref_xp_credit" {
		return errors.New("invalid credit voucher type")
	}
	count, err := CountByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, credit_amount, source)
		VALUES ($1, $2, 'inactive', $3, $4);
	`, userID, voucherType, creditAmount, source)
	return err
}

func GrantCreditVoucherTx(ctx context.Context, tx pgx.Tx, userID int, voucherType string, creditAmount float64, source string, maxSlots int) error {
	if voucherType != "usdt_credit" && voucherType != "lavx_credit" && voucherType != "ref_xp_credit" {
		return errors.New("invalid credit voucher type")
	}
	count, err := countByUserIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, credit_amount, source)
		VALUES ($1, $2, 'inactive', $3, $4);
	`, userID, voucherType, creditAmount, source)
	return err
}
