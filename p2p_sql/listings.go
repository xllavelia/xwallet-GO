package p2p_sql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListingWithPoster struct {
	Listing
	Username       string
	PlayerID       string
	IsOfficial     bool
	TotalDeals     int
	CompletedDeals int
}

func CreateListing(ctx context.Context, pool *pgxpool.Pool, userID int, side string, assetClass string, baseAsset string, rateUsd float64, minUsd float64, maxUsd float64, baseAmount float64, note string) (int, error) {
	var isMerchant bool
	pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM p2p_merchants WHERE user_id=$1);`, userID).Scan(&isMerchant)
	if !isMerchant {
		return 0, ErrNotMerchant
	}
	if side != "buy" && side != "sell" {
		return 0, ErrInvalidListing
	}
	if !ValidateAsset(assetClass, baseAsset) {
		return 0, ErrInvalidListing
	}
	if rateUsd <= 0 || baseAmount <= 0 || minUsd <= 0 || maxUsd < minUsd {
		return 0, ErrInvalidListing
	}

	var fundingSource bankcards_sql.FundingSource
	var fsErr error
	if side == "buy" {
		fundingSource, fsErr = bankcards_sql.ResolveFundingSource(ctx, pool, userID)
		if fsErr != nil {
			return 0, fsErr
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if side == "sell" {
		if err := moveAssetOut(ctx, tx, userID, assetClass, baseAsset, baseAmount); err != nil {
			return 0, err
		}
	} else {
		usdNeeded := baseAmount * rateUsd
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -usdNeeded); err != nil {
			return 0, err
		}
	}

	var id int
	err = tx.QueryRow(ctx, `
		INSERT INTO p2p_listings (user_id, side, base_asset, asset_class, rate_usd, min_amount_usd, max_amount_usd, remaining_base_amount, payment_note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id;
	`, userID, side, baseAsset, assetClass, rateUsd, minUsd, maxUsd, baseAmount, note).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, tx.Commit(ctx)
}

func GetActiveListings(ctx context.Context, pool *pgxpool.Pool, side string, assetClass string, asset string, amountUsd float64, officialOnly bool, excludeUserID int) ([]ListingWithPoster, error) {
	sortDir := "ASC"
	if side == "buy" {
		sortDir = "DESC"
	}

	sqlQuery := `
	SELECT l.id, l.user_id, l.side, l.base_asset, l.asset_class, l.rate_usd, l.min_amount_usd, l.max_amount_usd, l.remaining_base_amount, l.payment_note, l.status, l.created_at,
	       u.username, u.player_id, u.is_admin,
	       COALESCE(m.total_deals,0), COALESCE(m.completed_deals,0)
	FROM p2p_listings l
	JOIN users u ON u.id = l.user_id
	LEFT JOIN p2p_merchants m ON m.user_id = l.user_id
	WHERE l.status = 'active' AND l.side = $1 AND l.user_id != $2 AND l.remaining_base_amount > 0.00000001
	`
	args := []interface{}{side, excludeUserID}
	argN := 3

	if assetClass != "" && assetClass != "all" {
		sqlQuery += fmt.Sprintf(" AND l.asset_class = $%d", argN)
		args = append(args, assetClass)
		argN++
	}
	if asset != "" {
		sqlQuery += fmt.Sprintf(" AND l.base_asset = $%d", argN)
		args = append(args, asset)
		argN++
	}
	if amountUsd > 0 {
		sqlQuery += fmt.Sprintf(" AND l.min_amount_usd <= $%d AND l.max_amount_usd >= $%d", argN, argN)
		args = append(args, amountUsd)
		argN++
	}
	if officialOnly {
		sqlQuery += " AND u.is_admin = true"
	}
	sqlQuery += " ORDER BY u.is_admin DESC, l.rate_usd " + sortDir + " LIMIT 40;"

	rows, err := pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []ListingWithPoster{}
	for rows.Next() {
		var l ListingWithPoster
		if err := rows.Scan(&l.ID, &l.UserID, &l.Side, &l.BaseAsset, &l.AssetClass, &l.RateUsd, &l.MinAmountUsd, &l.MaxAmountUsd, &l.RemainingBaseAmount, &l.PaymentNote, &l.Status, &l.CreatedAt, &l.Username, &l.PlayerID, &l.IsOfficial, &l.TotalDeals, &l.CompletedDeals); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

func GetMyListings(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Listing, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, side, base_asset, asset_class, rate_usd, min_amount_usd, max_amount_usd, remaining_base_amount, payment_note, status, created_at
		FROM p2p_listings WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Listing{}
	for rows.Next() {
		var l Listing
		if err := rows.Scan(&l.ID, &l.UserID, &l.Side, &l.BaseAsset, &l.AssetClass, &l.RateUsd, &l.MinAmountUsd, &l.MaxAmountUsd, &l.RemainingBaseAmount, &l.PaymentNote, &l.Status, &l.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

func getListingForUpdate(ctx context.Context, tx pgx.Tx, listingID int) (Listing, error) {
	var l Listing
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, side, base_asset, asset_class, rate_usd, min_amount_usd, max_amount_usd, remaining_base_amount, payment_note, status, created_at
		FROM p2p_listings WHERE id = $1 FOR UPDATE;
	`, listingID).Scan(&l.ID, &l.UserID, &l.Side, &l.BaseAsset, &l.AssetClass, &l.RateUsd, &l.MinAmountUsd, &l.MaxAmountUsd, &l.RemainingBaseAmount, &l.PaymentNote, &l.Status, &l.CreatedAt)
	return l, err
}

func CloseListing(ctx context.Context, pool *pgxpool.Pool, userID int, listingID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	l, err := getListingForUpdate(ctx, tx, listingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrListingNotFound
		}
		return err
	}
	if l.UserID != userID {
		return ErrListingNotFound
	}
	if l.Status != "active" {
		return nil
	}

	if l.RemainingBaseAmount > 0 {
		if l.Side == "sell" {
			if err := moveAssetIn(ctx, tx, userID, l.AssetClass, l.BaseAsset, l.RemainingBaseAmount, l.RateUsd); err != nil {
				return err
			}
		} else {
			usdRefund := l.RemainingBaseAmount * l.RateUsd
			fundingSource, fsErr := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
			if fsErr != nil {
				return fsErr
			}
			if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, usdRefund); err != nil {
				return err
			}
		}
	}

	_, err = tx.Exec(ctx, `UPDATE p2p_listings SET status='closed', remaining_base_amount=0 WHERE id=$1;`, listingID)
	if err != nil {
		return err
	}
	_ = time.Now
	return tx.Commit(ctx)
}
