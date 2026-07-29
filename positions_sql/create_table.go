package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreatePositionsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS positions(
		id SERIAL PRIMARY KEY,
		trade_id CHAR(10) NOT NULL UNIQUE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		coin VARCHAR(8) NOT NULL,
		type VARCHAR(8) NOT NULL,
		entry_price NUMERIC(18, 8) NOT NULL,
		close_price NUMERIC(18, 8),
		leverage INT NOT NULL,
		amount NUMERIC(14, 2) NOT NULL,
		margin NUMERIC(14, 2) NOT NULL,
		fees NUMERIC(14, 2) NOT NULL DEFAULT 0,
		fees_paid_by_voucher BOOLEAN NOT NULL DEFAULT false,
		liq_price NUMERIC(18, 8) NOT NULL,
		auto_close BOOLEAN NOT NULL DEFAULT false,
		auto_close_target NUMERIC(6, 2),
		pnl NUMERIC(14, 2),
		pnl_percent NUMERIC(8, 2),
		status VARCHAR(8) NOT NULL DEFAULT 'open',
		result VARCHAR(8),
		opened_at TIMESTAMP NOT NULL DEFAULT now(),
		closed_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_positions_user_status ON positions (user_id, status);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
