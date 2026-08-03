package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateReferralsSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE referrals DROP CONSTRAINT IF EXISTS referral_code_format;
	ALTER TABLE referrals ALTER COLUMN referral_code TYPE VARCHAR(8);
	ALTER TABLE referrals ADD COLUMN IF NOT EXISTS ref_xp INT NOT NULL DEFAULT 0;
	ALTER TABLE referrals DROP COLUMN IF EXISTS invited_count;
	ALTER TABLE referrals DROP COLUMN IF EXISTS active_referred_traders;
	ALTER TABLE referrals DROP COLUMN IF EXISTS referral_level;
	ALTER TABLE referrals DROP COLUMN IF EXISTS max_invites;
	ALTER TABLE referrals ADD CONSTRAINT referral_code_format CHECK (referral_code ~ '^REF-[A-Z]{2}[0-9]{2}$');
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
