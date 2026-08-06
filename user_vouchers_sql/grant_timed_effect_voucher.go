package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func GrantXpBoostVoucherTx(ctx context.Context, tx pgx.Tx, userID int, percent float64, durationSeconds int, source string, maxSlots int) error {
	count, err := countByUserIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, limit_amount, duration_seconds, source)
		VALUES ($1, 'xp_boost', 'inactive', $2, $3, $4);
	`, userID, percent, durationSeconds, source)
	return err
}

func GrantFeeBoostVoucherTx(ctx context.Context, tx pgx.Tx, userID int, pointsOff float64, durationSeconds int, source string, maxSlots int) error {
	count, err := countByUserIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, limit_amount, duration_seconds, source)
		VALUES ($1, 'fee_boost', 'inactive', $2, $3, $4);
	`, userID, pointsOff, durationSeconds, source)
	return err
}
