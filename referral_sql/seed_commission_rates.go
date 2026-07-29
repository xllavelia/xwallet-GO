package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Плейсхолдер-значения — подкрутишь позже под реальную экономику.
func SeedCommissionRates(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	INSERT INTO referral_commission_rates (tier, commission_percent) VALUES
		('free', 5.00),
		('pro', 10.00),
		('prime', 15.00),
		('star', 20.00)
	ON CONFLICT (tier) DO NOTHING;
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
