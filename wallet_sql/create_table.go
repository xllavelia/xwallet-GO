package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateWalletsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS wallets(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		balance NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
		profit_24h NUMERIC(14, 2) NOT NULL DEFAULT 0,
		profit_7d NUMERIC(14, 2) NOT NULL DEFAULT 0,
		active_trades_count INT NOT NULL DEFAULT 0,
		win_rate NUMERIC(5, 2) NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL DEFAULT now()
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
