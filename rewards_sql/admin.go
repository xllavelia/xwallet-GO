package rewards_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidTrack = errors.New("invalid track")

type AdminState struct {
	Track         string         `json:"track"`
	LastClaimedAt *time.Time     `json:"lastClaimedAt"`
	NextClaimAt   *time.Time     `json:"nextClaimAt"`
	Claims        map[string]int `json:"claims"`
	TotalDays     int            `json:"totalDays"`
}

func GetAdminState(ctx context.Context, pool *pgxpool.Pool, userID int) (*AdminState, error) {
	s := &AdminState{Track: "start", Claims: map[string]int{}, TotalDays: TrackDays}
	err := pool.QueryRow(ctx, `SELECT track, last_claimed_at FROM daily_rewards_progress WHERE user_id = $1`, userID).
		Scan(&s.Track, &s.LastClaimedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if s.LastClaimedAt != nil {
		next := s.LastClaimedAt.Add(Cooldown)
		s.NextClaimAt = &next
	}
	claims, err := countClaimsByTrack(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	for k, v := range claims {
		s.Claims[k] = v
	}
	return s, nil
}

// AdminReset стирает все отметки получения и запускает трек заново с первого дня без кулдауна.
func AdminReset(ctx context.Context, pool *pgxpool.Pool, userID int, track string) error {
	if CatalogFor(track) == nil {
		return ErrInvalidTrack
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM daily_reward_claims WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO daily_rewards_progress (user_id, track, last_claimed_at) VALUES ($1, $2, NULL)
ON CONFLICT (user_id) DO UPDATE SET track = EXCLUDED.track, last_claimed_at = NULL`, userID, track); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdminSkipCooldown делает следующую награду доступной сразу, не трогая прогресс.
func AdminSkipCooldown(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	_, err := pool.Exec(ctx, `UPDATE daily_rewards_progress SET last_claimed_at = NULL WHERE user_id = $1`, userID)
	return err
}
