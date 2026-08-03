package card_history_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateCardHistoryTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS card_history(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		operation_type VARCHAR(8) NOT NULL,
		from_asset VARCHAR(8) NOT NULL,
		to_asset VARCHAR(8) NOT NULL,
		from_amount NUMERIC(18, 8) NOT NULL,
		to_amount NUMERIC(18, 8) NOT NULL,
		price NUMERIC(18, 8) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT card_history_operation_type_check CHECK (operation_type IN ('buy', 'sell', 'swap'))
	);
	CREATE INDEX IF NOT EXISTS idx_card_history_user ON card_history (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
