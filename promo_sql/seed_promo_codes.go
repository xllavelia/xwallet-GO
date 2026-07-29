package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SeedPromoCodes(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	INSERT INTO promo_codes (code, reward_type, reward_value, reward_duration_days, max_uses) VALUES
		('x27a1m20sn2', 'usdt', 25.00, NULL, NULL),
		('q8pxr41vte3', 'voucher', 100.00, NULL, NULL),
		('z55klm2fbn7', 'xp', 500, NULL, NULL),
		('r19dqow3ptz', 'fee_discount', 0.10, 7, NULL),
		('m73vgxu6cle', 'referral_boost', 1, 30, 500)
	ON CONFLICT (code) DO NOTHING;
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
