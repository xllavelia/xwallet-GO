package slots_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateSlotsSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
CREATE TABLE IF NOT EXISTS slots_profiles(
	user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	spins INT NOT NULL DEFAULT 0,
	wager_pool DOUBLE PRECISION NOT NULL DEFAULT 0,
	total_spins INT NOT NULL DEFAULT 0,
	total_wins INT NOT NULL DEFAULT 0,
	total_winnings DOUBLE PRECISION NOT NULL DEFAULT 0,
	total_wagered DOUBLE PRECISION NOT NULL DEFAULT 0,
	biggest_win DOUBLE PRECISION NOT NULL DEFAULT 0,
	current_streak INT NOT NULL DEFAULT 0,
	best_streak INT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS slots_history(
	id BIGSERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	level_id VARCHAR(24) NOT NULL,
	reels TEXT NOT NULL,
	win DOUBLE PRECISION NOT NULL DEFAULT 0,
	event VARCHAR(24) NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_slots_history_user
	ON slots_history (user_id, id DESC);

CREATE TABLE IF NOT EXISTS slots_daily(
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	day DATE NOT NULL DEFAULT CURRENT_DATE,
	spins INT NOT NULL DEFAULT 0,
	wins INT NOT NULL DEFAULT 0,
	winnings DOUBLE PRECISION NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, day)
);`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
