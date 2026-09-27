package slots_sql

import (
	"context"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// round2 / round4 — округления для денег и номинала спина.
func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

type SlotsProfile struct {
	UserID        int
	Spins         int
	WagerPool     float64
	TotalSpins    int
	TotalWins     int
	TotalWinnings float64
	TotalWagered  float64
	BiggestWin    float64
	CurrentStreak int
	BestStreak    int
}

func GetOrCreateProfile(ctx context.Context, pool *pgxpool.Pool, userID int) (*SlotsProfile, error) {
	p := &SlotsProfile{UserID: userID}
	err := pool.QueryRow(ctx, `
SELECT spins, wager_pool, total_spins, total_wins, total_winnings,
       total_wagered, biggest_win, current_streak, best_streak
FROM slots_profiles WHERE user_id = $1;`, userID).
		Scan(&p.Spins, &p.WagerPool, &p.TotalSpins, &p.TotalWins, &p.TotalWinnings,
			&p.TotalWagered, &p.BiggestWin, &p.CurrentStreak, &p.BestStreak)
	if err == nil {
		return p, nil
	}
	_, err = pool.Exec(ctx, `
INSERT INTO slots_profiles (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;`, userID)
	return p, err
}

func BuildState(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]interface{}, error) {
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return nil, err
	}

	history := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT id, level_id, reels, win, event,
       extract(epoch from created_at)::bigint
FROM slots_history WHERE user_id = $1
ORDER BY id DESC LIMIT $2;`, userID, HistoryStateLimit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var levelID, reels, event string
			var win float64
			var ts int64
			if err := rows.Scan(&id, &levelID, &reels, &win, &event, &ts); err != nil {
				rows.Close()
				return nil, err
			}
			history = append(history, map[string]interface{}{
				"id": id, "level_id": levelID,
				"reels": strings.Split(reels, ","),
				"win":   round2(win), "event": event, "t": ts,
			})
		}
		rows.Close()
	}

	// Сегодняшняя статистика (0, если сегодня ещё не крутили).
	var spinsToday, winsToday int
	var winningsToday float64
	_ = pool.QueryRow(ctx, `
SELECT spins, wins, winnings FROM slots_daily
WHERE user_id = $1 AND day = CURRENT_DATE;`, userID).
		Scan(&spinsToday, &winsToday, &winningsToday)

	avgWin := 0.0
	if profile.TotalWins > 0 {
		avgWin = profile.TotalWinnings / float64(profile.TotalWins)
	}
	rtp := 0.0
	if profile.TotalWagered > 0 {
		rtp = profile.TotalWinnings / profile.TotalWagered * 100
	}
	// Подсказка номинала спина для фронта (средняя цена спина сейчас).
	stakeHint := 0.0
	if profile.Spins > 0 {
		stakeHint = profile.WagerPool / float64(profile.Spins)
	}

	return map[string]interface{}{
		"spins":      profile.Spins,
		"stake_hint": round2(stakeHint),
		"history":    history,
		"stats": map[string]interface{}{
			"total_spins":    profile.TotalSpins,
			"total_wins":     profile.TotalWins,
			"total_winnings": round2(profile.TotalWinnings),
			"total_wagered":  round2(profile.TotalWagered),
			"total_profit":   round2(profile.TotalWinnings - profile.TotalWagered),
			"biggest_win":    round2(profile.BiggestWin),
			"current_streak": profile.CurrentStreak,
			"best_streak":    profile.BestStreak,
			"rtp":            round2(rtp),
			"spins_today":    spinsToday,
			"wins_today":     winsToday,
			"winnings_today": round2(winningsToday),
			"average_win":    round2(avgWin),
		},
		"config": buildConfigDTO(),
	}, nil
}

func buildConfigDTO() map[string]interface{} {
	symbols := []map[string]interface{}{}
	for _, s := range SlotsSymbols {
		symbols = append(symbols, map[string]interface{}{
			"id": s.ID, "name": s.Name, "weight": s.Weight, "mult": s.Mult,
		})
	}
	combos := []map[string]interface{}{}
	for _, c := range SlotsCombos {
		combos = append(combos, map[string]interface{}{
			"reels": c.Reels, "mult": c.Mult,
		})
	}
	levels := []map[string]interface{}{}
	for _, l := range SlotsLevels {
		levels = append(levels, map[string]interface{}{
			"id": l.ID, "name": l.Name, "spins_cost": l.SpinsCost,
			"payout_mult": l.PayoutMult, "pool": l.Pool,
		})
	}
	packs := []map[string]interface{}{}
	for _, p := range SlotsPacks {
		packs = append(packs, map[string]interface{}{
			"id": p.ID, "name": p.Name, "spins": p.Spins, "price": round2(p.Price),
		})
	}
	events := []map[string]interface{}{}
	for _, e := range SlotsEvents {
		events = append(events, map[string]interface{}{
			"id": e.ID, "name": e.Name, "weight": e.Weight, "bonus_spins": e.BonusSpins,
		})
	}
	return map[string]interface{}{
		"symbols": symbols, "combos": combos, "levels": levels,
		"packs": packs, "events": events, "lines": SlotsLines,
	}
}
