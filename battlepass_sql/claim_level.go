package battlepass_sql

import (
	"context"
	"encoding/json"
	"errors"

	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLevelLocked = errors.New("level not unlocked yet")
var ErrAlreadyClaimed = errors.New("level already claimed")
var ErrNoActiveTrack = errors.New("no active battle pass track")
var ErrLevelNotFound = errors.New("level not found")

type ClaimResult struct {
	Components []RewardComponent
}

func ClaimLevel(ctx context.Context, pool *pgxpool.Pool, userID int, activeTier string, level int, maxSlots int) (ClaimResult, error) {
	progress, err := GetProgress(ctx, pool, userID, activeTier)
	if err != nil {
		return ClaimResult{}, err
	}
	if progress.Track == nil {
		return ClaimResult{}, ErrNoActiveTrack
	}

	catalog := CatalogForTrack(*progress.Track)
	var target *Level
	for i := range catalog {
		if catalog[i].Level == level {
			target = &catalog[i]
			break
		}
	}
	if target == nil {
		return ClaimResult{}, ErrLevelNotFound
	}

	if progress.Xp < level*XPPerLevel {
		return ClaimResult{}, ErrLevelLocked
	}
	for _, cl := range progress.ClaimedLevels {
		if cl == level {
			return ClaimResult{}, ErrAlreadyClaimed
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return ClaimResult{}, err
	}
	defer tx.Rollback(ctx)

	for _, comp := range target.Components {
		switch comp.Kind {
		case "usdt":
			if _, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;`, comp.Value, userID); err != nil {
				return ClaimResult{}, err
			}
		case "lavx":
			if _, err := tx.Exec(ctx, `UPDATE wallets SET lavx_balance = lavx_balance + $1 WHERE user_id = $2;`, comp.Value, userID); err != nil {
				return ClaimResult{}, err
			}
		case "ref_xp":
			if _, err := tx.Exec(ctx, `UPDATE referrals SET ref_xp = ref_xp + $1 WHERE user_id = $2;`, int(comp.Value), userID); err != nil {
				return ClaimResult{}, err
			}
		case "voucher_fee":
			err := user_vouchers_sql.GrantFeeDiscountVoucherTx(ctx, tx, userID, comp.Value, comp.Days*86400, "battlepass", maxSlots)
			if err != nil && err != user_vouchers_sql.ErrSlotsFull {
				return ClaimResult{}, err
			}
		case "xp_boost":
			err := user_vouchers_sql.GrantXpBoostVoucherTx(ctx, tx, userID, comp.Value, comp.Days*86400, "battlepass", maxSlots)
			if err != nil && err != user_vouchers_sql.ErrSlotsFull {
				return ClaimResult{}, err
			}
		case "fee_boost":
			err := user_vouchers_sql.GrantFeeBoostVoucherTx(ctx, tx, userID, comp.Value, comp.Days*86400, "battlepass", maxSlots)
			if err != nil && err != user_vouchers_sql.ErrSlotsFull {
				return ClaimResult{}, err
			}
		case "case":
			column := comp.Label + "_cases"
			sqlQuery := `UPDATE battlepass_progress SET ` + column + ` = ` + column + ` + 1 WHERE user_id = $1;`
			if _, err := tx.Exec(ctx, sqlQuery, userID); err != nil {
				return ClaimResult{}, err
			}
		case "status":
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_statuses (user_id, status) VALUES ($1, $2)
				ON CONFLICT (user_id, status) DO NOTHING;
			`, userID, comp.Label); err != nil {
				return ClaimResult{}, err
			}
		}
	}

	newClaimed := append(progress.ClaimedLevels, level)
	claimedJSON, _ := json.Marshal(newClaimed)
	if _, err := tx.Exec(ctx, `UPDATE battlepass_progress SET claimed_tiers = $1 WHERE user_id = $2;`, claimedJSON, userID); err != nil {
		return ClaimResult{}, err
	}

	return ClaimResult{Components: target.Components}, tx.Commit(ctx)
}
