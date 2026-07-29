package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateReferralLinksTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS referral_links(
		id SERIAL PRIMARY KEY,
		referrer_user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		referred_user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		UNIQUE(referred_user_id)
	);
	CREATE INDEX IF NOT EXISTS idx_referral_links_referrer ON referral_links (referrer_user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
