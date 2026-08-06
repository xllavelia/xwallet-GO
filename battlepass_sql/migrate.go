package battlepass_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateBattlepassSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS track VARCHAR(16);
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS epic_cases INT NOT NULL DEFAULT 0;
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS mythic_cases INT NOT NULL DEFAULT 0;
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS legendary_cases INT NOT NULL DEFAULT 0;
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS last_transfer_xp_at TIMESTAMP;
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS last_card_buy_xp_at TIMESTAMP;
	ALTER TABLE battlepass_progress ADD COLUMN IF NOT EXISTS last_card_sell_xp_at TIMESTAMP;
	ALTER TABLE battlepass_progress DROP COLUMN IF EXISTS is_premium;

	CREATE TABLE IF NOT EXISTS user_statuses(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		status VARCHAR(24) NOT NULL,
		granted_at TIMESTAMP NOT NULL DEFAULT now(),
		UNIQUE(user_id, status)
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}

func MigrateXpAwardedColumns(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS xp_awarded INT NOT NULL DEFAULT 0;
	ALTER TABLE transfers ADD COLUMN IF NOT EXISTS xp_awarded INT NOT NULL DEFAULT 0;
	ALTER TABLE card_history ADD COLUMN IF NOT EXISTS xp_awarded INT NOT NULL DEFAULT 0;
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
