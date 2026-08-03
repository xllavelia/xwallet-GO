package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateUserVouchersTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS user_vouchers(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		voucher_type VARCHAR(16) NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'inactive',

		limit_amount NUMERIC(10, 2),
		used_amount NUMERIC(10, 2) NOT NULL DEFAULT 0,
		duration_seconds INT,
		activated_at TIMESTAMP,

		credit_amount NUMERIC(14, 2),

		source VARCHAR(16) NOT NULL DEFAULT 'welcome',
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT user_vouchers_type_check CHECK (voucher_type IN ('fee_discount', 'usdt_credit', 'lavx_credit')),
		CONSTRAINT user_vouchers_status_check CHECK (status IN ('inactive', 'active'))
	);
	CREATE INDEX IF NOT EXISTS idx_user_vouchers_user ON user_vouchers (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
