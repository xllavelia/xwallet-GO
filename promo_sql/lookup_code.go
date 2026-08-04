package promo_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrCodeNotFound = errors.New("promo code not found")

func LookupCodeForUpdate(ctx context.Context, tx pgx.Tx, code string) (PromoCode, error) {
	var p PromoCode
	err := tx.QueryRow(ctx, `
		SELECT id, code, reward_type, reward_value, reward_duration_days, max_uses, is_active, expires_at, created_at
		FROM promo_codes
		WHERE code = $1
		FOR UPDATE;
	`, code).Scan(&p.ID, &p.Code, &p.RewardType, &p.RewardValue, &p.RewardDurationDays, &p.MaxUses, &p.IsActive, &p.ExpiresAt, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PromoCode{}, ErrCodeNotFound
		}
		return PromoCode{}, err
	}

	return p, nil
}

func CountRedemptions(ctx context.Context, tx pgx.Tx, promoCodeID int) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM promo_code_redemptions WHERE promo_code_id = $1;`, promoCodeID).Scan(&count)

	return count, err
}

func CountVoucherSlots(ctx context.Context, tx pgx.Tx, userID int) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count)

	return count, err
}

func TryRecordRedemption(ctx context.Context, tx pgx.Tx, promoCodeID int, userID int) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO promo_code_redemptions (promo_code_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (promo_code_id, user_id) DO NOTHING;
	`, promoCodeID, userID)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() > 0, nil
}
