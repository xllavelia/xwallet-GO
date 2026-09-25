package rewards_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateRewardsSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
CREATE TABLE IF NOT EXISTS daily_rewards_progress(
	user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	track VARCHAR(16) NOT NULL DEFAULT 'start',
	last_claimed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS daily_reward_claims(
	id BIGSERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	track VARCHAR(16) NOT NULL,
	day INT NOT NULL,
	claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (user_id, track, day)
);

CREATE INDEX IF NOT EXISTS idx_daily_reward_claims_user
	ON daily_reward_claims (user_id, track);
`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
