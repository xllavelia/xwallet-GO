package prime_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreatePrimeSubscriptionsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS prime_subscriptions(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		tier VARCHAR(16),
		billing VARCHAR(16),
		expires_at TIMESTAMP
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
