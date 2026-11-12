package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// LogVoucherClaim фиксирует погашение ваучера в журнале начислений.
func LogVoucherClaim(ctx context.Context, tx pgx.Tx, userID int, voucherType string, amount float64, source string) error {
	_, err := tx.Exec(ctx, `INSERT INTO voucher_claims_log (user_id, voucher_type, amount, source) VALUES ($1, $2, $3, $4);`, userID, voucherType, amount, source)
	return err
}
