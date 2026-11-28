package user_vouchers_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSlotsFull = errors.New("all voucher slots are full")

const voucherSlotsAdvisoryLockNamespace = 1735390081

func CountByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)
	return count, err
}

func countByUserIDTx(ctx context.Context, tx pgx.Tx, userID int) (int, error) {
	// Проверка лимита и вставка должны быть сериализованы между всеми источниками ваучеров.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1::integer, $2::integer);`, voucherSlotsAdvisoryLockNamespace, userID); err != nil {
		return 0, err
	}
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)
	return count, err
}

func GrantFeeDiscountVoucher(ctx context.Context, pool *pgxpool.Pool, userID int, limitAmount float64, durationSeconds int, source string, maxSlots int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := GrantFeeDiscountVoucherTx(ctx, tx, userID, limitAmount, durationSeconds, source, maxSlots); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := GrantCreditVoucherTx(ctx, tx, userID, voucherType, creditAmount, source, maxSlots); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
