package commodities_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateCommoditiesSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS commodity_holdings(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		symbol VARCHAR(10) NOT NULL,
		quantity NUMERIC(18, 8) NOT NULL DEFAULT 0,
		avg_cost NUMERIC(14, 4) NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL DEFAULT now(),
		UNIQUE(user_id, symbol)
	);
	CREATE INDEX IF NOT EXISTS idx_commodity_holdings_user ON commodity_holdings (user_id);

	CREATE TABLE IF NOT EXISTS commodity_history(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		symbol VARCHAR(10) NOT NULL,
		operation_type VARCHAR(8) NOT NULL,
		usd_amount NUMERIC(14, 2) NOT NULL,
		quantity NUMERIC(18, 8) NOT NULL,
		price NUMERIC(14, 4) NOT NULL,
		realized_pnl NUMERIC(14, 2),
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT commodity_history_op_check CHECK (operation_type IN ('buy', 'sell'))
	);
	CREATE INDEX IF NOT EXISTS idx_commodity_history_user ON commodity_history (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
