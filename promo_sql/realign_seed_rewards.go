package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Старые тестовые коды были засеяны до того, как появились реальные
// Voucher/Referral системы. Приводит их к тому, что реально работает сейчас.
func RealignSeedRewards(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	UPDATE promo_codes SET code = UPPER(code);

	UPDATE promo_codes SET reward_type = 'usdt', reward_value = 25.00
		WHERE code = 'X27A1M20SN2';
	UPDATE promo_codes SET reward_type = 'usdt_voucher', reward_value = 100.00
		WHERE code = 'Q8PXR41VTE3';
	UPDATE promo_codes SET reward_type = 'ref_xp_voucher', reward_value = 50.00
		WHERE code = 'Z55KLM2FBN7';
	UPDATE promo_codes SET reward_type = 'fee_voucher', reward_value = 50.00, reward_duration_days = 7
		WHERE code = 'R19DQOW3PTZ';
	UPDATE promo_codes SET reward_type = 'ref_xp_voucher', reward_value = 100.00
		WHERE code = 'M73VGXU6CLE';
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
