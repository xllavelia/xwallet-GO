package p2p_sql

import (
	"context"
	"errors"
	"time"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateDeal(ctx context.Context, pool *pgxpool.Pool, takerUserID int, listingID int, quoteAmountUsd float64) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	l, err := getListingForUpdate(ctx, tx, listingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrListingNotFound
		}
		return 0, err
	}
	if l.Status != "active" {
		return 0, ErrListingNotFound
	}
	if l.UserID == takerUserID {
		return 0, ErrCannotDealOwnListing
	}
	if quoteAmountUsd < l.MinAmountUsd || quoteAmountUsd > l.MaxAmountUsd {
		return 0, ErrAmountOutOfRange
	}
	baseAmount := quoteAmountUsd / l.RateUsd
	if baseAmount > l.RemainingBaseAmount {
		return 0, ErrListingInsufficientVolume
	}

	if _, err := tx.Exec(ctx, `UPDATE p2p_listings SET remaining_base_amount = remaining_base_amount - $1 WHERE id=$2;`, baseAmount, listingID); err != nil {
		return 0, err
	}

	commissionUsd := quoteAmountUsd * 0.01
	expiresAt := time.Now().Add(time.Duration(DealExpiryMinutes) * time.Minute)

	var dealID int
	err = tx.QueryRow(ctx, `
		INSERT INTO p2p_deals (listing_id, poster_user_id, taker_user_id, side, base_asset, asset_class, base_amount, quote_amount_usd, rate_usd, commission_usd, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'awaiting_payment',$11) RETURNING id;
	`, listingID, l.UserID, takerUserID, l.Side, l.BaseAsset, l.AssetClass, baseAmount, quoteAmountUsd, l.RateUsd, commissionUsd, expiresAt).Scan(&dealID)
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `UPDATE p2p_merchants SET total_deals = total_deals + 1 WHERE user_id=$1;`, l.UserID); err != nil {
		return 0, err
	}

	return dealID, tx.Commit(ctx)
}

func getDealForUpdate(ctx context.Context, tx pgx.Tx, dealID int) (Deal, error) {
	var d Deal
	err := tx.QueryRow(ctx, `
		SELECT id, listing_id, poster_user_id, taker_user_id, side, base_asset, asset_class, base_amount, quote_amount_usd, rate_usd, commission_usd, status, expires_at, completed_at, created_at
		FROM p2p_deals WHERE id=$1 FOR UPDATE;
	`, dealID).Scan(&d.ID, &d.ListingID, &d.PosterUserID, &d.TakerUserID, &d.Side, &d.BaseAsset, &d.AssetClass, &d.BaseAmount, &d.QuoteAmountUsd, &d.RateUsd, &d.CommissionUsd, &d.Status, &d.ExpiresAt, &d.CompletedAt, &d.CreatedAt)
	return d, err
}

func ConfirmDeal(ctx context.Context, pool *pgxpool.Pool, actingUserID int, dealID int) error {
	posterFundingCache := map[int]bankcards_sql.FundingSource{}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	d, err := getDealForUpdate(ctx, tx, dealID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDealNotFound
		}
		return err
	}
	if d.TakerUserID != actingUserID && d.PosterUserID != actingUserID {
		return ErrDealNotFound
	}
	if d.Status != "awaiting_payment" {
		return ErrDealNotPending
	}
	if time.Now().After(d.ExpiresAt) {
		return ErrDealExpired
	}

	posterFunding, err := bankcards_sql.ResolveFundingSource(ctx, pool, d.PosterUserID)
	if err != nil {
		return err
	}
	takerFunding, err := bankcards_sql.ResolveFundingSource(ctx, pool, d.TakerUserID)
	if err != nil {
		return err
	}
	posterFundingCache[d.PosterUserID] = posterFunding

	if d.Side == "sell" {
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, takerFunding, -d.QuoteAmountUsd); err != nil {
			return err
		}
		posterReceivesUsd := d.QuoteAmountUsd - d.CommissionUsd
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, posterFunding, posterReceivesUsd); err != nil {
			return err
		}
		if err := moveAssetIn(ctx, tx, d.TakerUserID, d.AssetClass, d.BaseAsset, d.BaseAmount, d.RateUsd); err != nil {
			return err
		}
	} else {
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, takerFunding, d.QuoteAmountUsd); err != nil {
			return err
		}
		if err := moveAssetOut(ctx, tx, d.TakerUserID, d.AssetClass, d.BaseAsset, d.BaseAmount); err != nil {
			return err
		}
		commissionBase := 0.0
		if d.RateUsd > 0 {
			commissionBase = d.CommissionUsd / d.RateUsd
		}
		posterReceivesBase := d.BaseAmount - commissionBase
		if posterReceivesBase < 0 {
			posterReceivesBase = 0
		}
		if err := moveAssetIn(ctx, tx, d.PosterUserID, d.AssetClass, d.BaseAsset, posterReceivesBase, d.RateUsd); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE p2p_deals SET status='completed', completed_at=now() WHERE id=$1;`, dealID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE p2p_merchants SET completed_deals = completed_deals + 1 WHERE user_id=$1;`, d.PosterUserID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func CancelDeal(ctx context.Context, pool *pgxpool.Pool, actingUserID int, dealID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	d, err := getDealForUpdate(ctx, tx, dealID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDealNotFound
		}
		return err
	}
	if actingUserID != d.PosterUserID && actingUserID != d.TakerUserID {
		return ErrDealNotFound
	}
	if d.Status != "awaiting_payment" {
		return ErrDealNotPending
	}

	if _, err := tx.Exec(ctx, `UPDATE p2p_listings SET remaining_base_amount = remaining_base_amount + $1 WHERE id=$2;`, d.BaseAmount, d.ListingID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE p2p_deals SET status='cancelled' WHERE id=$1;`, dealID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type DealWithParty struct {
	Deal
	PosterUsername string
	TakerUsername  string
}

func GetMyDeals(ctx context.Context, pool *pgxpool.Pool, userID int, activeOnly bool) ([]DealWithParty, error) {
	statusFilter := ""
	if activeOnly {
		statusFilter = "AND d.status = 'awaiting_payment'"
	} else {
		statusFilter = "AND d.status != 'awaiting_payment'"
	}
	sqlQuery := `
	SELECT d.id, d.listing_id, d.poster_user_id, d.taker_user_id, d.side, d.base_asset, d.asset_class, d.base_amount, d.quote_amount_usd, d.rate_usd, d.commission_usd, d.status, d.expires_at, d.completed_at, d.created_at,
	       pu.username, tu.username
	FROM p2p_deals d
	JOIN users pu ON pu.id = d.poster_user_id
	JOIN users tu ON tu.id = d.taker_user_id
	WHERE (d.poster_user_id = $1 OR d.taker_user_id = $1) ` + statusFilter + `
	ORDER BY d.created_at DESC LIMIT 60;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DealWithParty{}
	for rows.Next() {
		var d DealWithParty
		if err := rows.Scan(&d.ID, &d.ListingID, &d.PosterUserID, &d.TakerUserID, &d.Side, &d.BaseAsset, &d.AssetClass, &d.BaseAmount, &d.QuoteAmountUsd, &d.RateUsd, &d.CommissionUsd, &d.Status, &d.ExpiresAt, &d.CompletedAt, &d.CreatedAt, &d.PosterUsername, &d.TakerUsername); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func GetDealDetail(ctx context.Context, pool *pgxpool.Pool, userID int, dealID int) (DealWithParty, error) {
	var d DealWithParty
	err := pool.QueryRow(ctx, `
		SELECT d.id, d.listing_id, d.poster_user_id, d.taker_user_id, d.side, d.base_asset, d.asset_class, d.base_amount, d.quote_amount_usd, d.rate_usd, d.commission_usd, d.status, d.expires_at, d.completed_at, d.created_at,
		       pu.username, tu.username
		FROM p2p_deals d
		JOIN users pu ON pu.id = d.poster_user_id
		JOIN users tu ON tu.id = d.taker_user_id
		WHERE d.id = $1 AND (d.poster_user_id = $2 OR d.taker_user_id = $2);
	`, dealID, userID).Scan(&d.ID, &d.ListingID, &d.PosterUserID, &d.TakerUserID, &d.Side, &d.BaseAsset, &d.AssetClass, &d.BaseAmount, &d.QuoteAmountUsd, &d.RateUsd, &d.CommissionUsd, &d.Status, &d.ExpiresAt, &d.CompletedAt, &d.CreatedAt, &d.PosterUsername, &d.TakerUsername)
	if err != nil {
		return DealWithParty{}, ErrDealNotFound
	}
	return d, nil
}

func StartExpiryWorker(pool *pgxpool.Pool) {
	go func() {
		for {
			expireStaleDeals(pool)
			time.Sleep(1 * time.Minute)
		}
	}()
}

func expireStaleDeals(pool *pgxpool.Pool) {
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT id FROM p2p_deals WHERE status='awaiting_payment' AND expires_at < now();`)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	for _, id := range ids {
		tx, err := pool.Begin(ctx)
		if err != nil {
			continue
		}
		d, err := getDealForUpdate(ctx, tx, id)
		if err != nil || d.Status != "awaiting_payment" {
			tx.Rollback(ctx)
			continue
		}
		tx.Exec(ctx, `UPDATE p2p_listings SET remaining_base_amount = remaining_base_amount + $1 WHERE id=$2;`, d.BaseAmount, d.ListingID)
		tx.Exec(ctx, `UPDATE p2p_deals SET status='expired' WHERE id=$1;`, id)
		tx.Commit(ctx)
	}
}
