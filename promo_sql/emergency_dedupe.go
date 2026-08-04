package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EmergencyDedupeAndFix(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	
	SELECT pg_terminate_backend(pid)
	FROM pg_stat_activity
	WHERE state = 'idle in transaction'
	  AND now() - state_change > interval '30 seconds'
	  AND pid != pg_backend_pid();


	UPDATE promo_codes SET code = UPPER(code);

	
	DELETE FROM promo_code_redemptions
	WHERE promo_code_id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY code ORDER BY id ASC) AS rn
			FROM promo_codes
		) ranked WHERE rn > 1
	);


	DELETE FROM promo_codes
	WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY code ORDER BY id ASC) AS rn
			FROM promo_codes
		) ranked WHERE rn > 1
	);

	
	CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_codes_code_unique ON promo_codes (code);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
