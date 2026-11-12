package user_vouchers_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CustomVoucherTypes — одноразовые ваучеры на платную кастомизацию карточки.
var CustomVoucherTypes = map[string]bool{
	"custom_free": true,
	"custom_20":   true,
	"custom_50":   true,
}

// CustomVoucherDiscount возвращает скидку в процентах (0–100) для типа ваучера.
func CustomVoucherDiscount(voucherType string) float64 {
	switch voucherType {
	case "custom_free":
		return 100
	case "custom_20":
		return 20
	case "custom_50":
		return 50
	}
	return 0
}

func GrantCustomVoucherTx(ctx context.Context, tx pgx.Tx, userID int, voucherType, source string, maxSlots int) error {
	if !CustomVoucherTypes[voucherType] {
		return errors.New("invalid custom voucher type")
	}
	count, err := countByUserIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if count >= maxSlots {
		return ErrSlotsFull
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_vouchers (user_id, voucher_type, status, source)
		VALUES ($1, $2, 'inactive', $3);
	`, userID, voucherType, source)
	return err
}

// ConsumeCustomVoucherTx одноразово погашает неактивный кастом-ваучер и возвращает его скидку в процентах.
func ConsumeCustomVoucherTx(ctx context.Context, tx pgx.Tx, userID, id int) (float64, error) {
	var voucherType string
	err := tx.QueryRow(ctx, `
		DELETE FROM user_vouchers
		WHERE id = $1 AND user_id = $2 AND voucher_type IN ('custom_free', 'custom_20', 'custom_50') AND status = 'inactive'
		RETURNING voucher_type;
	`, id, userID).Scan(&voucherType)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("custom voucher not found")
	}
	if err != nil {
		return 0, err
	}
	return CustomVoucherDiscount(voucherType), nil
}
