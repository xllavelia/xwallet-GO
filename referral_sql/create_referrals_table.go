package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateReferralsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS referrals(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		referral_code CHAR(6) NOT NULL UNIQUE,
		invited_count INT NOT NULL DEFAULT 0,
		active_referred_traders INT NOT NULL DEFAULT 0,
		referral_level INT NOT NULL DEFAULT 1,
		max_invites INT NOT NULL DEFAULT 10,
		total_referred_volume NUMERIC(16, 2) NOT NULL DEFAULT 0,
		total_earned NUMERIC(14, 2) NOT NULL DEFAULT 0,

		CONSTRAINT referral_code_format CHECK (referral_code ~ '^[a-z][0-9]{2}[a-z][0-9]{2}$')
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
