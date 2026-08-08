package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateVoucherTypesForRefXP(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
 BEGIN;

 ALTER TABLE user_vouchers
 DROP CONSTRAINT IF EXISTS user_vouchers_type_check;

 ALTER TABLE user_vouchers
 ADD CONSTRAINT user_vouchers_type_check
 CHECK (
  voucher_type IN (
   'fee_boost',
   'fee_discount',
   'usdt_credit',
   'lavx_credit',
   'ref_xp_credit'
  )
 );

 COMMIT;
 `

	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
