package rewards_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/prime_sql"
	"xwallet-server/user_vouchers_sql"
)

var (
	ErrNoTrack        = errors.New("no active track")
	ErrTrackFinished  = errors.New("track finished")
	ErrCooldown       = errors.New("next reward is not ready yet")
	ErrAlreadyClaimed = errors.New("reward already claimed")
)

type ClaimResult struct {
	Day              int               `json:"day"`
	Track            string            `json:"track"`
	Granted          []RewardComponent `json:"granted"`
	FinishedTrack    bool              `json:"finishedTrack"`
	SecondsRemaining int               `json:"secondsRemaining"`
}

func ClaimDailyReward(ctx context.Context, pool *pgxpool.Pool, userID int) (*ClaimResult, error) {
	maxSlots, err := prime_sql.GetMaxVoucherSlots(ctx, pool, userID)
	if err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
INSERT INTO daily_rewards_progress (user_id, track)
VALUES ($1, 'start')
ON CONFLICT (user_id) DO NOTHING;
`, userID); err != nil {
		return nil, err
	}

	var track string
	var lastClaimed *time.Time
	if err = tx.QueryRow(ctx, `
SELECT track, last_claimed_at FROM daily_rewards_progress WHERE user_id = $1 FOR UPDATE;
`, userID).Scan(&track, &lastClaimed); err != nil {
		return nil, err
	}

	catalog := CatalogFor(track)
	if catalog == nil {
		return nil, ErrNoTrack
	}

	if lastClaimed != nil && time.Now().Before(lastClaimed.Add(Cooldown)) {
		return nil, ErrCooldown
	}

	var claimsCount int
	if err = tx.QueryRow(ctx, `
SELECT COUNT(*) FROM daily_reward_claims WHERE user_id = $1 AND track = $2;
`, userID, track).Scan(&claimsCount); err != nil {
		return nil, err
	}

	day := claimsCount + 1
	if day > len(catalog) {
		if _, err = tx.Exec(ctx, `
UPDATE daily_rewards_progress SET track = 'none' WHERE user_id = $1;
`, userID); err != nil {
			return nil, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrTrackFinished
	}

	ct, err := tx.Exec(ctx, `
INSERT INTO daily_reward_claims (user_id, track, day)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, track, day) DO NOTHING;
`, userID, track, day)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrAlreadyClaimed
	}

	reward := catalog[day-1]
	granted := make([]RewardComponent, 0, len(reward.Components))
	for _, comp := range reward.Components {
		switch comp.Kind {
		case "usdt":
			_, err = tx.Exec(ctx, `
UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;
`, comp.Value, userID)
		case "lavx":
			_, err = tx.Exec(ctx, `
UPDATE wallets SET lavx_balance = lavx_balance + $1 WHERE user_id = $2;
`, comp.Value, userID)
		case "voucher":
			if err = user_vouchers_sql.GrantCreditVoucherTx(ctx, tx, userID, "usdt_credit", comp.Value, "daily_rewards", maxSlots); err == user_vouchers_sql.ErrSlotsFull {
				err = nil
			}
		case "ref_xp":
			_, err = tx.Exec(ctx, `
UPDATE referrals SET ref_xp = ref_xp + $1 WHERE user_id = $2;
`, int64(comp.Value), userID)
		case "pass_xp":
			_, err = tx.Exec(ctx, `
INSERT INTO battlepass_progress (user_id, xp)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET xp = battlepass_progress.xp + $2;
`, userID, int64(comp.Value))
		default:
			err = nil
		}
		if err != nil {
			return nil, err
		}
		granted = append(granted, comp)
	}

	finished := day >= len(catalog)
	newTrack := track
	if finished {
		newTrack = "none"
	}
	if _, err = tx.Exec(ctx, `
UPDATE daily_rewards_progress SET track = $2, last_claimed_at = now() WHERE user_id = $1;
`, userID, newTrack); err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &ClaimResult{
		Day:              day,
		Track:            track,
		Granted:          granted,
		FinishedTrack:    finished,
		SecondsRemaining: int(Cooldown.Seconds()),
	}, nil
}
