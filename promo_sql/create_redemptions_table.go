package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateRedemptionsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS promo_code_redemptions(
		id SERIAL PRIMARY KEY,
		promo_code_id INT NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		UNIQUE(promo_code_id, user_id)
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
