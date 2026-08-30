package savings_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateSavingsTables(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS savings_accounts(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		balance NUMERIC(14, 2) NOT NULL DEFAULT 0,
		last_accrued_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE TABLE IF NOT EXISTS savings_history(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		entry_type VARCHAR(16) NOT NULL,
		amount NUMERIC(14, 2) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT savings_history_type_check CHECK (entry_type IN ('deposit', 'withdrawal', 'interest'))
	);
	CREATE INDEX IF NOT EXISTS idx_savings_history_user ON savings_history (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
