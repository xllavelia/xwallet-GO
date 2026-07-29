package voucher_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateVouchersTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS vouchers(
		id SERIAL PRIMARY KEY,
		voucher_code VARCHAR(10) NOT NULL UNIQUE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		limit_amount NUMERIC(10, 2) NOT NULL DEFAULT 400.00,
		used_amount NUMERIC(10, 2) NOT NULL DEFAULT 0,
		trades_covered INT NOT NULL DEFAULT 0,
		status VARCHAR(16) NOT NULL DEFAULT 'active',
		source VARCHAR(16) NOT NULL DEFAULT 'default',
		activated_at TIMESTAMP,
		expires_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_vouchers_user ON vouchers (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
