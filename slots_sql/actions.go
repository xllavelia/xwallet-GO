package slots_sql

import (
	"context"
	"errors"
	"math/rand"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/wallet_sql"
)

// ============================================================
// СПРАВОЧНИКИ
// ============================================================

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

// validLines — число линий из запроса: не прислали (0) -> 1,
// иначе только значение из SlotsLines.
func validLines(lines int) (int, error) {
	if lines <= 0 {
		return 1, nil
	}
	for _, l := range SlotsLines {
		if l == lines {
			return lines, nil
		}
	}
	return 0, errors.New("unknown lines")
}

// eventBonus — сколько бесплатных спинов даёт событие (0 для не-бонусных).
func eventBonus(eventID string) int {
	for _, e := range SlotsEvents {
		if e.ID == eventID {
			return e.BonusSpins
		}
	}
	return 0
}

// ============================================================
// МАГАЗИН
// ============================================================

// ActionBuyPack — покупка набора спинов: деньги списываются с кошелька
// и уходят в wager_pool, так у каждого спина честная средняя цена.
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

// ============================================================
// РОЗЫГРЫШ
// ============================================================

// rollReel — один барабан по весам символов пула уровня.
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

// comboMult — выплата за комбинацию. Сначала три одинаковых символа
// (явное правило из таблицы или mult символа), потом wildcard-правила
// сверху вниз.
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

// lineResult — итог одной линии.
type lineResult struct {
	reels [3]string
	win   float64
	event string
}

// rollLine — розыгрыш одной линии: символы, выплата и редкое событие.
// lucky_spin / double_reward перекручивают барабаны до выигрыша,
// у double_reward выплата удваивается.
func rollLine(level *SlotsLevel, stake float64) lineResult {
	reels := rollAll(level)
	win := round2(comboMult(reels) * level.PayoutMult * stake)

	eventID := ""
	r := rand.Float64()
	for _, e := range SlotsEvents {
		if r < e.Weight {
			eventID = e.ID
			break
		}
		r -= e.Weight
	}

	switch eventID {
	case "lucky_spin", "double_reward":
		for t := 0; t < EventRerollMax && win <= 0; t++ {
			reels = rollAll(level)
			win = round2(comboMult(reels) * level.PayoutMult * stake)
		}
		if eventID == "double_reward" && win > 0 {
			win = round2(win * 2)
		}
	}

	return lineResult{reels: reels, win: win, event: eventID}
}

// ============================================================
// СПИН
// ============================================================

// ActionSpin — спин на lines строк. Стоимость = SpinsCost уровня × lines
// и списывается одним атомарным UPDATE. Дальше lines независимых
// розыгрышей: у каждой строки свои барабаны, своё событие, своя выплата
// и своя запись в истории. Номинал строки считается из пула на её момент,
// поэтому экономика идентична lines последовательным нажатиям рычага.
func ActionSpin(ctx context.Context, pool *pgxpool.Pool, userID int, levelID string, lines int) error {
	level := findLevel(levelID)
	if level == nil {
		return errors.New("unknown level")
	}
	lines, err := validLines(lines)
	if err != nil {
		return err
	}

	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	cost := level.SpinsCost * lines
	if profile.Spins < cost {
		return errors.New("not enough spins — buy a pack in the shop")
	}

	// Атомарное списание стоимости ВСЕХ линий — защита от двойного клика.
	var spinsLeft int
	var poolLeft float64
	err = pool.QueryRow(ctx, `
UPDATE slots_profiles SET spins = spins - $2
WHERE user_id = $1 AND spins >= $2
RETURNING spins, wager_pool;`, userID, cost).Scan(&spinsLeft, &poolLeft)
	if err != nil {
		return errors.New("not enough spins — buy a pack in the shop")
	}

	// spinsLeft/poolLeft — уже после списания; возвращаем cost, чтобы счётчик
	// линий стартовал с предспинового баланса.
	spinsRun := spinsLeft + cost
	poolRun := poolLeft

	results := make([]lineResult, 0, lines)
	totalWin := 0.0
	totalWagered := 0.0
	wins := 0
	bonusSpins := 0
	streak := profile.CurrentStreak
	bestStreak := profile.BestStreak
	biggestWin := profile.BiggestWin

	for i := 0; i < lines; i++ {
		// Номинал строки — средняя цена спина из пула ставок. Если пул пуст
		// (игра на бонусных спинах) — крутим по MinStakeUSDT за счёт кассы.
		stake := MinStakeUSDT
		if poolRun > 0 && spinsRun > 0 {
			stake = poolRun / float64(spinsRun)
			if stake < MinStakeUSDT {
				stake = MinStakeUSDT
			}
		}
		stake = round4(stake)

		res := rollLine(level, stake)

		// Спины уже списаны одним UPDATE, здесь гасим только пул.
		spinsRun -= level.SpinsCost
		poolRun -= stake
		if poolRun < 0 {
			poolRun = 0
		}

		totalWagered += stake
		totalWin += res.win
		bonusSpins += eventBonus(res.event)
		if res.win > 0 {
			wins++
			streak++
			if res.win > biggestWin {
				biggestWin = res.win
			}
		} else {
			streak = 0
		}
		if streak > bestStreak {
			bestStreak = streak
		}
		results = append(results, res)
	}

	totalWin = round2(totalWin)
	totalWagered = round2(totalWagered)

	// Суммарная выплата на кошелёк одной проводкой.
	if totalWin > 0 {
		if err := wallet_sql.AdjustBalance(ctx, pool, userID, totalWin); err != nil {
			return err
		}
	}

	// Бонусные спины событий (могли выпасть сразу на нескольких линиях).
	if bonusSpins > 0 {
		if _, err := pool.Exec(ctx, `
UPDATE slots_profiles SET spins = spins + $2 WHERE user_id = $1;`,
			userID, bonusSpins); err != nil {
			return err
		}
	}

	if _, err := pool.Exec(ctx, `
UPDATE slots_profiles SET
    wager_pool = $2, total_spins = total_spins + $3, total_wins = total_wins + $4,
    total_winnings = total_winnings + $5, total_wagered = total_wagered + $6,
    biggest_win = $7, current_streak = $8, best_streak = $9
WHERE user_id = $1;`,
		userID, poolRun, lines, wins, totalWin, totalWagered,
		biggestWin, streak, bestStreak); err != nil {
		return err
	}

	// Дневная статистика: каждая линия считается за спин.
	if _, err := pool.Exec(ctx, `
INSERT INTO slots_daily (user_id, day, spins, wins, winnings)
VALUES ($1, CURRENT_DATE, $2, $3, $4)
ON CONFLICT (user_id, day) DO UPDATE SET
    spins = slots_daily.spins + EXCLUDED.spins,
    wins = slots_daily.wins + EXCLUDED.wins,
    winnings = slots_daily.winnings + EXCLUDED.winnings;`,
		userID, lines, wins, totalWin); err != nil {
		return err
	}

	// По записи истории на каждую линию, в порядке строк машины сверху вниз
	// (фронт читает history DESC и разворачивает обратно).
	for _, res := range results {
		if _, err := pool.Exec(ctx, `
INSERT INTO slots_history (user_id, level_id, reels, win, event, lines)
VALUES ($1, $2, $3, $4, $5, $6);`,
			userID, level.ID, res.reels[0]+","+res.reels[1]+","+res.reels[2],
			res.win, res.event, lines); err != nil {
			return err
		}
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
