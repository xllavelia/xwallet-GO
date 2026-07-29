package battlepass_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateBattlepassProgressTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS battlepass_progress(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		xp INT NOT NULL DEFAULT 0,
		is_premium BOOLEAN NOT NULL DEFAULT false,
		claimed_tiers JSONB NOT NULL DEFAULT '[]',
		season_id INT NOT NULL DEFAULT 1
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
