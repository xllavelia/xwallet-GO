package referral_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateCommissionRatesTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS referral_commission_rates(
		tier VARCHAR(16) PRIMARY KEY,
		commission_percent NUMERIC(5, 2) NOT NULL
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
