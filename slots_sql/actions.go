package slots_sql

import (
	"context"
	"errors"
	"math/rand"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/wallet_sql"
)

func findPack(packID string) *SlotsPack {
	for i := range SlotsPacks {
		if SlotsPacks[i].ID == packID {
			return &SlotsPacks[i]
		}
	}
	return nil
}

func findLevel(levelID string) *SlotsLevel {
	for i := range SlotsLevels {
		if SlotsLevels[i].ID == levelID {
			return &SlotsLevels[i]
		}
	}
	return nil
}

// Покупка набора спинов: деньги списываются с кошелька и уходят
// в wager_pool — так у каждого спина честная средняя цена.
func ActionBuyPack(ctx context.Context, pool *pgxpool.Pool, userID int, packID string) error {
	pack := findPack(packID)
	if pack == nil {
		return errors.New("unknown pack")
	}
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < pack.Price {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -pack.Price); err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
UPDATE slots_profiles
SET spins = spins + $2, wager_pool = wager_pool + $3
WHERE user_id = $1;`, userID, pack.Spins, pack.Price)
	return err
}

// Один барабан по весам символов пула уровня.
func rollReel(level *SlotsLevel) string {
	total := 0.0
	weights := make([]float64, len(level.Pool))
	for i, id := range level.Pool {
		w := 1.0
		if s := SlotsSymbolByID[id]; s != nil {
			w = s.Weight
		}
		weights[i] = w
		total += w
	}
	r := rand.Float64() * total
	for i, w := range weights {
		if r < w {
			return level.Pool[i]
		}
		r -= w
	}
	return level.Pool[len(level.Pool)-1]
}

func rollAll(level *SlotsLevel) [3]string {
	var reels [3]string
	for i := 0; i < 3; i++ {
		reels[i] = rollReel(level)
	}
	return reels
}

// Выплата за комбинацию. Сначала три одинаковых символа (явное правило
// из таблицы или mult символа), потом wildcard-правила сверху вниз.
func comboMult(reels [3]string) float64 {
	if reels[0] == reels[1] && reels[1] == reels[2] {
		for _, c := range SlotsCombos {
			if c.Reels[0] == reels[0] && c.Reels[1] == reels[1] && c.Reels[2] == reels[2] {
				return c.Mult
			}
		}
		if s := SlotsSymbolByID[reels[0]]; s != nil {
			return s.Mult
		}
		return 0
	}
	for _, c := range SlotsCombos {
		match := true
		for i := 0; i < 3; i++ {
			if c.Reels[i] != "" && c.Reels[i] != reels[i] {
				match = false
				break
			}
		}
		if match {
			return c.Mult
		}
	}
	return 0
}

// Собственно спин: списание спинов, розыгрыш, события, выплата, статистика.
func ActionSpin(ctx context.Context, pool *pgxpool.Pool, userID int, levelID string) error {
	level := findLevel(levelID)
	if level == nil {
		return errors.New("unknown level")
	}

	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	if profile.Spins < level.SpinsCost {
		return errors.New("not enough spins — buy a pack in the shop")
	}

	// Номинал спина — средняя цена из пула ставок. Если пул пуст
	// (только бонусные спины) — играем по MinStakeUSDT за счёт кассы.
	stake := MinStakeUSDT
	if profile.WagerPool > 0 && profile.Spins > 0 {
		stake = profile.WagerPool / float64(profile.Spins)
		if stake < MinStakeUSDT {
			stake = MinStakeUSDT
		}
	}
	stake = round4(stake)

	// Атомарное списание — защита от двойного клика/гонок.
	var left int
	err = pool.QueryRow(ctx, `
UPDATE slots_profiles SET spins = spins - $2
WHERE user_id = $1 AND spins >= $2
RETURNING spins;`, userID, level.SpinsCost).Scan(&left)
	if err != nil {
		return errors.New("not enough spins — buy a pack in the shop")
	}

	reels := rollAll(level)
	win := round2(comboMult(reels) * level.PayoutMult * stake)

	// Редкое событие: один бросок на все события, не больше одного за спин.
	eventID := ""
	eventBonusSpins := 0
	r := rand.Float64()
	for _, e := range SlotsEvents {
		if r < e.Weight {
			eventID = e.ID
			eventBonusSpins = e.BonusSpins
			break
		}
		r -= e.Weight
	}
	switch eventID {
	case "lucky_spin":
		for t := 0; t < EventRerollMax && win <= 0; t++ {
			reels = rollAll(level)
			win = round2(comboMult(reels) * level.PayoutMult * stake)
		}
	case "double_reward":
		for t := 0; t < EventRerollMax && win <= 0; t++ {
			reels = rollAll(level)
			win = round2(comboMult(reels) * level.PayoutMult * stake)
		}
		if win > 0 {
			win = round2(win * 2)
		}
	case "bonus_round":
		if eventBonusSpins > 0 {
			if _, err := pool.Exec(ctx, `
UPDATE slots_profiles SET spins = spins + $2 WHERE user_id = $1;`,
				userID, eventBonusSpins); err != nil {
				return err
			}
		}
	}

	// Выплата на баланс кошелька.
	if win > 0 {
		if err := wallet_sql.AdjustBalance(ctx, pool, userID, win); err != nil {
			return err
		}
	}

	// Стрики.
	newStreak := 0
	if win > 0 {
		newStreak = profile.CurrentStreak + 1
	}
	bestStreak := profile.BestStreak
	if newStreak > bestStreak {
		bestStreak = newStreak
	}
	biggestWin := profile.BiggestWin
	if win > biggestWin {
		biggestWin = win
	}
	totalWins := profile.TotalWins
	if win > 0 {
		totalWins++
	}
	wagerPool := profile.WagerPool - stake
	if wagerPool < 0 {
		wagerPool = 0
	}

	_, err = pool.Exec(ctx, `
UPDATE slots_profiles SET
    wager_pool = $2, total_spins = total_spins + 1, total_wins = $3,
    total_winnings = total_winnings + $4, total_wagered = total_wagered + $5,
    biggest_win = $6, current_streak = $7, best_streak = $8
WHERE user_id = $1;`,
		userID, wagerPool, totalWins, win, stake, biggestWin, newStreak, bestStreak)
	if err != nil {
		return err
	}

	// Дневная статистика.
	winFlag := 0
	if win > 0 {
		winFlag = 1
	}
	_, err = pool.Exec(ctx, `
INSERT INTO slots_daily (user_id, day, spins, wins, winnings)
VALUES ($1, CURRENT_DATE, 1, $2, $3)
ON CONFLICT (user_id, day) DO UPDATE SET
    spins = slots_daily.spins + 1,
    wins = slots_daily.wins + EXCLUDED.wins,
    winnings = slots_daily.winnings + EXCLUDED.winnings;`,
		userID, winFlag, win)
	if err != nil {
		return err
	}

	// История + подчистка старого.
	_, err = pool.Exec(ctx, `
INSERT INTO slots_history (user_id, level_id, reels, win, event)
VALUES ($1, $2, $3, $4, $5);`,
		userID, level.ID, reels[0]+","+reels[1]+","+reels[2], win, eventID)
	if err != nil {
		return err
	}
	_, _ = pool.Exec(ctx, `
DELETE FROM slots_history
WHERE user_id = $1
  AND id NOT IN (
      SELECT id FROM slots_history
      WHERE user_id = $1 ORDER BY id DESC LIMIT $2
  );`, userID, HistoryKeepLimit)

	return nil
}
