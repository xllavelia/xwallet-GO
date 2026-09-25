package rewards_sql

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DayDTO struct {
	Day        int               `json:"day"`
	Claimed    bool              `json:"claimed"`
	Components []RewardComponent `json:"components"`
}

type RewardsStats struct {
	TotalDays       int     `json:"totalDays"`
	ClaimedDays     int     `json:"claimedDays"`
	EarnedUsdt      float64 `json:"earnedUsdt"`
	EarnedLavx      float64 `json:"earnedLavx"`
	EarnedRefXp     int     `json:"earnedRefXp"`
	EarnedPassXp    int     `json:"earnedPassXp"`
	VouchersGranted int     `json:"vouchersGranted"`
}

type StateResponse struct {
	Track            string            `json:"track"`
	FinishedTrack    string            `json:"finishedTrack"`
	Days             []DayDTO          `json:"days"`
	NextDay          int               `json:"nextDay"`
	NextReward       []RewardComponent `json:"nextReward"`
	CanClaim         bool              `json:"canClaim"`
	NextClaimAt      *time.Time        `json:"nextClaimAt"`
	SecondsRemaining int               `json:"secondsRemaining"`
	Stats            RewardsStats      `json:"stats"`
}

type progressRow struct {
	Track         string
	LastClaimedAt *time.Time
}

func getOrCreateProgress(ctx context.Context, pool *pgxpool.Pool, userID int) (progressRow, error) {
	var p progressRow
	_, err := pool.Exec(ctx, `
INSERT INTO daily_rewards_progress (user_id, track)
VALUES ($1, 'start')
ON CONFLICT (user_id) DO NOTHING;
`, userID)
	if err != nil {
		return p, err
	}
	err = pool.QueryRow(ctx, `
SELECT track, last_claimed_at FROM daily_rewards_progress WHERE user_id = $1;
`, userID).Scan(&p.Track, &p.LastClaimedAt)
	return p, err
}

func countClaimsByTrack(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]int, error) {
	rows, err := pool.Query(ctx, `
SELECT track, COUNT(*) FROM daily_reward_claims WHERE user_id = $1 GROUP BY track;
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		m[t] = c
	}
	return m, rows.Err()
}

func buildStats(catalog []DayReward, claimed int) RewardsStats {
	s := RewardsStats{TotalDays: len(catalog), ClaimedDays: claimed}
	if claimed > len(catalog) {
		claimed = len(catalog)
	}
	for i := 0; i < claimed; i++ {
		for _, c := range catalog[i].Components {
			switch c.Kind {
			case "usdt":
				s.EarnedUsdt += c.Value
			case "voucher":
				s.VouchersGranted++
				s.EarnedUsdt += c.Value
			case "lavx":
				s.EarnedLavx += c.Value
			case "ref_xp":
				s.EarnedRefXp += int(c.Value)
			case "pass_xp":
				s.EarnedPassXp += int(c.Value)
			}
		}
	}
	return s
}

func GetState(ctx context.Context, pool *pgxpool.Pool, userID int) (*StateResponse, error) {
	p, err := getOrCreateProgress(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	claims, err := countClaimsByTrack(ctx, pool, userID)
	if err != nil {
		return nil, err
	}

	resp := &StateResponse{Track: p.Track}

	if p.Track == "none" {
		for _, t := range []string{"pro", "start"} {
			if claims[t] >= TrackDays {
				resp.FinishedTrack = t
				break
			}
		}
	}

	displayTrack := p.Track
	if p.Track == "none" && resp.FinishedTrack != "" {
		displayTrack = resp.FinishedTrack
	}

	catalog := CatalogFor(displayTrack)
	if catalog == nil {
		return resp, nil
	}

	claimed := claims[displayTrack]
	if claimed > len(catalog) {
		claimed = len(catalog)
	}

	days := make([]DayDTO, 0, len(catalog))
	for _, dr := range catalog {
		days = append(days, DayDTO{Day: dr.Day, Claimed: dr.Day <= claimed, Components: dr.Components})
	}
	resp.Days = days

	now := time.Now()
	canClaim := true
	if p.LastClaimedAt != nil {
		next := p.LastClaimedAt.Add(Cooldown)
		resp.NextClaimAt = &next
		if now.Before(next) {
			canClaim = false
			resp.SecondsRemaining = int(next.Sub(now).Seconds())
		}
	}

	if claimed >= len(catalog) {
		resp.NextDay = 0
		canClaim = false
		if p.Track != "none" {
			resp.FinishedTrack = p.Track
		}
	} else {
		resp.NextDay = claimed + 1
		resp.NextReward = catalog[claimed].Components
	}
	resp.CanClaim = canClaim
	resp.Stats = buildStats(catalog, claimed)

	return resp, nil
}
