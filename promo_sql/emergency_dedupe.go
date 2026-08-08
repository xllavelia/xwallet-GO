package promo_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EmergencyDedupeAndFix(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
 BEGIN;
 SELECT pg_terminate_backend(pid)
 FROM pg_stat_activity
 WHERE state = 'idle in transaction'
   AND now() - state_change > interval '30 seconds'
   AND pid != pg_backend_pid();


 UPDATE promo_code_redemptions r
 SET promo_code_id = p.keep_id
 FROM (
  SELECT
   id,
   FIRST_VALUE(id) OVER (
    PARTITION BY UPPER(code)
    ORDER BY id
   ) AS keep_id
  FROM promo_codes
 ) p
 WHERE r.promo_code_id = p.id
   AND p.id <> p.keep_id;


 DELETE FROM promo_codes p
 USING (
  SELECT id
  FROM (
   SELECT
    id,
    ROW_NUMBER() OVER (
     PARTITION BY UPPER(code)
     ORDER BY id ASC
    ) AS rn
   FROM promo_codes
  ) ranked
  WHERE rn > 1
 ) d
 WHERE p.id = d.id;


 UPDATE promo_codes
 SET code = UPPER(code)
 WHERE code <> UPPER(code);


 CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_codes_code_unique
 ON promo_codes (code);


 COMMIT;
 `

	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
