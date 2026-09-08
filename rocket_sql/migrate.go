package rocket_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateRocketSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS rocket_rounds(
		id SERIAL PRIMARY KEY,
		crash_point NUMERIC(6,2) NOT NULL,
		started_at TIMESTAMP,
		ended_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE TABLE IF NOT EXISTS rocket_bets(
		id SERIAL PRIMARY KEY,
		round_id INT NOT NULL REFERENCES rocket_rounds(id) ON DELETE CASCADE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		amount NUMERIC(14,2) NOT NULL,
		status VARCHAR(12) NOT NULL DEFAULT 'active',
		cashout_multiplier NUMERIC(6,2),
		payout NUMERIC(14,2),
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT rocket_bets_status_check CHECK (status IN ('active','cashed_out','lost')),
		UNIQUE(round_id, user_id)
	);
	CREATE INDEX IF NOT EXISTS idx_rocket_bets_round ON rocket_bets (round_id);
	CREATE INDEX IF NOT EXISTS idx_rocket_bets_user ON rocket_bets (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
