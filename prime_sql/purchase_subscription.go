package prime_sql

import (
	"context"
	"errors"
	"time"

	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidTier = errors.New("invalid tier")
var ErrInvalidBilling = errors.New("invalid billing")
var ErrInsufficientLavx = errors.New("insufficient LAVX balance")

func PurchaseSubscription(ctx context.Context, pool *pgxpool.Pool, userID int, tierID string, billing string) error {
	cfg, ok := Tiers[tierID]
	if !ok {
		return ErrInvalidTier
	}
	if billing != "monthly" && billing != "annual" {
		return ErrInvalidBilling
	}

	var cost float64
	var durationDays int
	if billing == "monthly" {
		cost = cfg.MonthlyPriceLavx
		durationDays = 30
	} else {
		cost = cfg.AnnualPriceLavx * 12
		durationDays = 365
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var newLavx float64
	err = tx.QueryRow(ctx, `
		UPDATE wallets SET lavx_balance = lavx_balance - $1
		WHERE user_id = $2 AND lavx_balance >= $1
		RETURNING lavx_balance;
	`, cost, userID).Scan(&newLavx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientLavx
		}
		return err
	}

	expiresAt := time.Now().AddDate(0, 0, durationDays)

	_, err = tx.Exec(ctx, `
		INSERT INTO prime_subscriptions (user_id, tier, billing, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET tier = $2, billing = $3, expires_at = $4;
	`, userID, tierID, billing, expiresAt)
	if err != nil {
		return err
	}

	if cfg.UsdVoucherAmount > 0 {
		err := user_vouchers_sql.GrantCreditVoucherTx(ctx, tx, userID, "usdt_credit", cfg.UsdVoucherAmount, "prime_"+tierID, cfg.MaxVoucherSlots)
		if err != nil && err != user_vouchers_sql.ErrSlotsFull {
			return err
		}
	}
	if cfg.FeeVoucherLimit > 0 {
		err := user_vouchers_sql.GrantFeeDiscountVoucherTx(ctx, tx, userID, cfg.FeeVoucherLimit, cfg.FeeVoucherDays*86400, "prime_"+tierID, cfg.MaxVoucherSlots)
		if err != nil && err != user_vouchers_sql.ErrSlotsFull {
			return err
		}
	}
	if cfg.RefXpVoucher > 0 {
		err := user_vouchers_sql.GrantCreditVoucherTx(ctx, tx, userID, "ref_xp_credit", cfg.RefXpVoucher, "prime_"+tierID, cfg.MaxVoucherSlots)
		if err != nil && err != user_vouchers_sql.ErrSlotsFull {
			return err
		}
	}

	return tx.Commit(ctx)
}
