package stocks_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateStocksSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS stock_holdings(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		symbol VARCHAR(8) NOT NULL,
		quantity NUMERIC(18, 8) NOT NULL DEFAULT 0,
		avg_cost NUMERIC(14, 4) NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL DEFAULT now(),
		UNIQUE(user_id, symbol)
	);
	CREATE INDEX IF NOT EXISTS idx_stock_holdings_user ON stock_holdings (user_id);

	CREATE TABLE IF NOT EXISTS stock_history(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		symbol VARCHAR(8) NOT NULL,
		operation_type VARCHAR(8) NOT NULL,
		usd_amount NUMERIC(14, 2) NOT NULL,
		quantity NUMERIC(18, 8) NOT NULL,
		price NUMERIC(14, 4) NOT NULL,
		realized_pnl NUMERIC(14, 2),
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT stock_history_op_check CHECK (operation_type IN ('buy', 'sell'))
	);
	CREATE INDEX IF NOT EXISTS idx_stock_history_user ON stock_history (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
