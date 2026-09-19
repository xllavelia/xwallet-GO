package pixel_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigratePixelSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS pixel_rounds(
		id SERIAL PRIMARY KEY,
		started_at TIMESTAMP,
		ended_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS pixel_boards(
		id SERIAL PRIMARY KEY,
		round_id INT NOT NULL REFERENCES pixel_rounds(id) ON DELETE CASCADE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		amount NUMERIC(14,2) NOT NULL,
		grid_cols INT NOT NULL,
		grid_rows INT NOT NULL,
		mine_positions JSONB NOT NULL,
		revealed_cells JSONB NOT NULL DEFAULT '[]',
		lives_remaining INT NOT NULL,
		status VARCHAR(12) NOT NULL DEFAULT 'active',
		payout NUMERIC(14,2),
		created_at TIMESTAMP NOT NULL DEFAULT now(),
		ended_at TIMESTAMP,

		CONSTRAINT pixel_boards_status_check CHECK (status IN ('active','cashed_out','lost','expired')),
		UNIQUE(round_id, user_id)
	);
	CREATE INDEX IF NOT EXISTS idx_pixel_boards_user ON pixel_boards (user_id);
	CREATE INDEX IF NOT EXISTS idx_pixel_boards_round ON pixel_boards (round_id);

	CREATE TABLE IF NOT EXISTS pixel_user_stats(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		total_cells_opened INT NOT NULL DEFAULT 0,
		safe_cells_opened INT NOT NULL DEFAULT 0,
		total_games INT NOT NULL DEFAULT 0,
		games_won INT NOT NULL DEFAULT 0,
		total_profit NUMERIC(14,2) NOT NULL DEFAULT 0
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
