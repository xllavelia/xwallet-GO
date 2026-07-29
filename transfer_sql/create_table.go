package transfer_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateTransfersTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS transfers(
		id SERIAL PRIMARY KEY,
		sender_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		recipient_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		amount NUMERIC(14, 2) NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'completed',
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_transfers_sender ON transfers (sender_id);
	CREATE INDEX IF NOT EXISTS idx_transfers_recipient ON transfers (recipient_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
