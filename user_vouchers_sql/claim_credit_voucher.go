package user_vouchers_sql

import (
	"context"
	"errors"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrVoucherNotFound = errors.New("voucher not found or already used")

func ClaimCreditVoucher(ctx context.Context, pool *pgxpool.Pool, id int, userID int) (string, float64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)

	var voucherType string
	var amount float64
	var originSource string

	err = tx.QueryRow(ctx, `
		DELETE FROM user_vouchers
		WHERE id = $1 AND user_id = $2 AND voucher_type IN ('usdt_credit', 'lavx_credit', 'ref_xp_credit')
		RETURNING voucher_type, credit_amount, source;
	`, id, userID).Scan(&voucherType, &amount, &originSource)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrVoucherNotFound
		}
		return "", 0, err
	}

	if voucherType == "usdt_credit" {
		_, err = tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;`, amount, userID)
	} else if voucherType == "lavx_credit" {
		_, err = tx.Exec(ctx, `UPDATE wallets SET lavx_balance = lavx_balance + $1 WHERE user_id = $2;`, amount, userID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE referrals SET ref_xp = ref_xp + $1 WHERE user_id = $2;`, int(amount), userID)
	}
	if err != nil {
		return "", 0, err
	}

	if err := bankcards_sql.LogVoucherClaim(ctx, tx, userID, voucherType, amount, originSource); err != nil {
		return "", 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}

	return voucherType, amount, nil
}
