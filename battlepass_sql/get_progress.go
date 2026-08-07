package battlepass_sql

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureRow(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO battlepass_progress (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING;
	`, userID)
	return err
}

func GetProgress(ctx context.Context, pool *pgxpool.Pool, userID int, activeTier string) (Progress, error) {
	if err := ensureRow(ctx, pool, userID); err != nil {
		return Progress{}, err
	}

	var p Progress
	var track *string
	var claimedRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT track, xp, claimed_tiers, classico_cases, elysium_cases, legendary_cases,
		       last_transfer_xp_at, last_card_buy_xp_at, last_card_sell_xp_at
		FROM battlepass_progress WHERE user_id = $1;
	`, userID).Scan(&track, &p.Xp, &claimedRaw, &p.ClassicoCases, &p.ElysiumCases, &p.LegendaryCases,
		&p.LastTransferXpAt, &p.LastCardBuyXpAt, &p.LastCardSellXpAt)
	if err != nil {
		return Progress{}, err
	}

	json.Unmarshal(claimedRaw, &p.ClaimedLevels)
	p.UserID = userID

	currentTrack := ""
	if track != nil {
		currentTrack = *track
	}

	if activeTier != "" && currentTrack != activeTier {
		_, err := pool.Exec(ctx, `UPDATE battlepass_progress SET track = $1, claimed_tiers = '[]' WHERE user_id = $2;`, activeTier, userID)
		if err != nil {
			return Progress{}, err
		}
		currentTrack = activeTier
		p.ClaimedLevels = []int{}
	} else if activeTier == "" {
		currentTrack = ""
	}

	if currentTrack == "" {
		p.Track = nil
	} else {
		p.Track = &currentTrack
	}

	return p, nil
}
