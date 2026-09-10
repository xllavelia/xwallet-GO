package p2p_sql

import (
	"context"
	"errors"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetMerchantStatus(ctx context.Context, pool *pgxpool.Pool, userID int) (*Merchant, error) {
	var m Merchant
	err := pool.QueryRow(ctx, `SELECT user_id, is_verified, deposit_amount, total_deals, completed_deals, created_at FROM p2p_merchants WHERE user_id=$1;`, userID).
		Scan(&m.UserID, &m.IsVerified, &m.DepositAmount, &m.TotalDeals, &m.CompletedDeals, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func BecomeMerchant(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	existing, err := GetMerchantStatus(ctx, pool, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrAlreadyMerchant
	}

	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -DepositAmountUsd); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return ErrInsufficientDeposit
		}
		return err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO p2p_merchants (user_id, is_verified, deposit_amount) VALUES ($1, true, $2);`, userID, DepositAmountUsd); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func WithdrawMerchantDeposit(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	var activeCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM p2p_listings WHERE user_id=$1 AND status='active';`, userID).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount > 0 {
		return ErrHasActiveListings
	}

	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deposit float64
	if err := tx.QueryRow(ctx, `SELECT deposit_amount FROM p2p_merchants WHERE user_id=$1 FOR UPDATE;`, userID).Scan(&deposit); err != nil {
		return err
	}
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, deposit); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM p2p_merchants WHERE user_id=$1;`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
