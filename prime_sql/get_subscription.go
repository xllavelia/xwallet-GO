package prime_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ActiveSubscription struct {
	Tier      string
	Billing   string
	ExpiresAt time.Time
}

func GetActiveSubscription(ctx context.Context, pool *pgxpool.Pool, userID int) (*ActiveSubscription, error) {
	var s ActiveSubscription
	err := pool.QueryRow(ctx, `
		SELECT tier, billing, expires_at FROM prime_subscriptions
		WHERE user_id = $1 AND tier IS NOT NULL AND expires_at > now();
	`, userID).Scan(&s.Tier, &s.Billing, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func GetEffectiveFeeRate(ctx context.Context, pool *pgxpool.Pool, userID int) (string, float64, error) {
	sub, err := GetActiveSubscription(ctx, pool, userID)
	if err != nil {
		return "", BaseFeeRatePercent, err
	}
	if sub == nil {
		return "", BaseFeeRatePercent, nil
	}
	cfg, ok := Tiers[sub.Tier]
	if !ok {
		return "", BaseFeeRatePercent, nil
	}
	return sub.Tier, cfg.FeeRatePercent, nil
}

func GetMaxVoucherSlots(ctx context.Context, pool *pgxpool.Pool, userID int) (int, error) {
	sub, err := GetActiveSubscription(ctx, pool, userID)
	if err != nil {
		return DefaultVoucherSlots, err
	}
	if sub == nil {
		return DefaultVoucherSlots, nil
	}
	cfg, ok := Tiers[sub.Tier]
	if !ok {
		return DefaultVoucherSlots, nil
	}
	return cfg.MaxVoucherSlots, nil
}
