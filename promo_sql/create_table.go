package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreatePromoCodesTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS promo_codes(
		id SERIAL PRIMARY KEY,
		code VARCHAR(20) NOT NULL UNIQUE,
		reward_type VARCHAR(24) NOT NULL,
		reward_value NUMERIC(14, 2) NOT NULL,
		reward_duration_days INT,
		max_uses INT,
		used_count INT NOT NULL DEFAULT 0,
		is_active BOOLEAN NOT NULL DEFAULT true,
		expires_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
