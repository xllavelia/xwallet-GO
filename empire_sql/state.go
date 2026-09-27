package empire_sql

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/wallet_sql"
)

// ============================================================
// ПРОФИЛЬ ИМПЕРИИ
// ============================================================

type EmpireProfile struct {
	UserID         int
	Level          int
	XP             float64
	PrestigeCount  int
	PrestigePoints int
	RP             float64
	TotalEarned    float64
	AllTimeEarned  float64
	TotalSpent     float64
	LastTick       time.Time
}

func GetOrCreateProfile(ctx context.Context, pool *pgxpool.Pool, userID int) (*EmpireProfile, error) {
	p := &EmpireProfile{UserID: userID}
	err := pool.QueryRow(ctx, `
SELECT level, xp, prestige_count, prestige_points, rp,
       total_earned, all_time_earned, total_spent, last_tick
FROM empire_profiles WHERE user_id = $1;`, userID).
		Scan(&p.Level, &p.XP, &p.PrestigeCount, &p.PrestigePoints, &p.RP,
			&p.TotalEarned, &p.AllTimeEarned, &p.TotalSpent, &p.LastTick)
	if err == nil {
		return p, nil
	}
	_, err = pool.Exec(ctx, `
INSERT INTO empire_profiles (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;`, userID)
	if err != nil {
		return nil, err
	}
	p.Level = 1
	p.LastTick = time.Now()
	return p, nil
}

func saveProfile(ctx context.Context, pool *pgxpool.Pool, p *EmpireProfile) error {
	_, err := pool.Exec(ctx, `
UPDATE empire_profiles
SET level=$2, xp=$3, prestige_count=$4, prestige_points=$5, rp=$6,
    total_earned=$7, all_time_earned=$8, total_spent=$9, last_tick=$10
WHERE user_id=$1;`,
		p.UserID, p.Level, p.XP, p.PrestigeCount, p.PrestigePoints, p.RP,
		p.TotalEarned, p.AllTimeEarned, p.TotalSpent, p.LastTick)
	return err
}

// applyXp добавляет XP и поднимает уровни (xp в профиле — прогресс
// внутри текущего уровня).
func applyXp(p *EmpireProfile, gain float64) {
	if gain <= 0 {
		return
	}
	p.XP += gain
	for {
		need := EmpireXpForLevel(p.Level)
		if p.XP < need {
			return
		}
		p.XP -= need
		p.Level++
	}
}

// ============================================================
// ЭКЗЕМПЛЯРЫ ОБЪЕКТОВ / ИССЛЕДОВАНИЯ / БУСТЕРЫ / АКЦИИ
// ============================================================

type ObjectInstance struct {
	ID             int
	DefID          string
	CustomName     string
	Level          int
	WorkersHired   int
	WorkerXP       float64
	Satisfaction   float64
	Invested       float64
	LifetimeEarned float64
	PurchasedAt    time.Time
}

func LoadObjects(ctx context.Context, pool *pgxpool.Pool, userID int) ([]ObjectInstance, error) {
	rows, err := pool.Query(ctx, `
SELECT id, def_id, COALESCE(custom_name, ''), level, workers_hired,
       worker_xp, satisfaction, invested, lifetime_earned, purchased_at
FROM empire_objects WHERE user_id = $1 ORDER BY id;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ObjectInstance{}
	for rows.Next() {
		var o ObjectInstance
		if err := rows.Scan(&o.ID, &o.DefID, &o.CustomName, &o.Level,
			&o.WorkersHired, &o.WorkerXP, &o.Satisfaction, &o.Invested,
			&o.LifetimeEarned, &o.PurchasedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func LoadResearch(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]int, error) {
	rows, err := pool.Query(ctx, `
SELECT direction, level FROM empire_research WHERE user_id = $1;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var dir string
		var lvl int
		if err := rows.Scan(&dir, &lvl); err != nil {
			return nil, err
		}
		out[dir] = lvl
	}
	return out, rows.Err()
}

func LoadBoosters(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]time.Time, error) {
	rows, err := pool.Query(ctx, `
SELECT booster_type, expires_at FROM empire_boosters
WHERE user_id = $1 AND expires_at > now();`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var typ string
		var exp time.Time
		if err := rows.Scan(&typ, &exp); err != nil {
			return nil, err
		}
		out[typ] = exp
	}
	return out, rows.Err()
}

type AssetHolding struct {
	Shares          int
	AvgPrice        float64
	DividendsEarned float64
}

func LoadHoldings(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]AssetHolding, error) {
	rows, err := pool.Query(ctx, `
SELECT asset_id, shares, avg_price, dividends_earned
FROM empire_asset_holdings WHERE user_id = $1 AND shares > 0;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]AssetHolding{}
	for rows.Next() {
		var id string
		var h AssetHolding
		if err := rows.Scan(&id, &h.Shares, &h.AvgPrice, &h.DividendsEarned); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

func loadAssetPrices(ctx context.Context, pool *pgxpool.Pool) (map[string]float64, error) {
	rows, err := pool.Query(ctx, `SELECT asset_id, price FROM empire_asset_prices;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var id string
		var p float64
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// ============================================================
// БОНУСЫ (исследования + уровень + престиж + бустеры)
// ============================================================

type EmpireBonuses struct {
	ObjectIncomeMult   float64 // множитель дохода объектов
	WorkerMult         float64 // множитель бонуса работников
	ExpensesMult       float64 // множитель расходов (< 1 — экономия)
	DividendsMult      float64 // множитель дивидендов
	ContractRewardMult float64 // множитель наград контрактов
	OfflineCapHours    float64 // лимит офлайн-начисления в часах
}

func boosterActive(boosters map[string]time.Time, typ string, now time.Time) bool {
	t, ok := boosters[typ]
	return ok && t.After(now)
}

func ComputeBonuses(p *EmpireProfile, research map[string]int, boosters map[string]time.Time, now time.Time) EmpireBonuses {
	b := EmpireBonuses{
		ObjectIncomeMult:   1 + LevelIncomeBonusPerLevel*float64(p.Level-1) + PrestigeIncomeBonusPerPP*float64(p.PrestigePoints),
		WorkerMult:         1,
		ExpensesMult:       1,
		DividendsMult:      1,
		ContractRewardMult: 1,
		OfflineCapHours:    OfflineCapHours,
	}
	if l := research["automation"]; l > 0 {
		b.ObjectIncomeMult += EmpireResearchByID["automation"].EffectPerLevel * float64(l)
	}
	if l := research["ai"]; l > 0 {
		b.WorkerMult += EmpireResearchByID["ai"].EffectPerLevel * float64(l)
	}
	if l := research["energy"]; l > 0 {
		b.ExpensesMult -= EmpireResearchByID["energy"].EffectPerLevel * float64(l)
		if b.ExpensesMult < 0.4 {
			b.ExpensesMult = 0.4
		}
	}
	if l := research["finance"]; l > 0 {
		b.DividendsMult += EmpireResearchByID["finance"].EffectPerLevel * float64(l)
	}
	if l := research["trading"]; l > 0 {
		b.ContractRewardMult += EmpireResearchByID["trading"].EffectPerLevel * float64(l)
	}
	if l := research["security"]; l > 0 {
		b.OfflineCapHours *= 1 + EmpireResearchByID["security"].EffectPerLevel*float64(l)
	}
	if boosterActive(boosters, "income_x2", now) {
		b.ObjectIncomeMult *= 2
	}
	if boosterActive(boosters, "workers_x2", now) {
		b.WorkerMult *= 2
	}
	if boosterActive(boosters, "expenses_half", now) {
		b.ExpensesMult *= 0.5
	}
	return b
}

// ============================================================
// РАСЧЁТ ПО ОБЪЕКТУ
// ============================================================

func workerQuality(o ObjectInstance) float64 {
	productivity := 1 + math.Min(WorkerMaxExpBonus, o.WorkerXP*WorkerExpRate)
	satisfactionFactor := o.Satisfaction / 100
	return productivity * satisfactionFactor
}

type ObjectStats struct {
	IncomePerHour   float64
	ExpensesPerHour float64
	NetPerHour      float64
	CurrentValue    float64 // BaseValueAt(level) + 60% вложенного в апгрейды
	SellValue       float64 // invested * SellBackRate
	Efficiency      float64 // NetPerHour / CurrentValue (часовая доходность)
	PaybackHours    float64 // CurrentValue / NetPerHour
	UpgradeCost     float64 // цена перехода на следующий уровень (0 = макс)
	WorkerQuality   float64
	RPHour          float64 // RP/час (только research-сектор)
}

func ComputeObjectStats(def EmpireObjectDef, o ObjectInstance, b EmpireBonuses, rpBooster bool) ObjectStats {
	quality := workerQuality(o)
	workerBonus := 1 + WorkerIncomeBonusPerWorker*float64(o.WorkersHired)*quality*b.WorkerMult
	income := def.IncomeAt(o.Level) * workerBonus * b.ObjectIncomeMult
	expenses := def.ExpensesAt(o.Level) * b.ExpensesMult
	net := income - expenses
	baseValue := def.BaseValueAt(o.Level)
	upgradesInvested := math.Max(0, o.Invested-def.BaseCost)
	currentValue := baseValue + 0.6*upgradesInvested

	st := ObjectStats{
		IncomePerHour:   income,
		ExpensesPerHour: expenses,
		NetPerHour:      net,
		CurrentValue:    currentValue,
		SellValue:       o.Invested * SellBackRate,
		UpgradeCost:     def.UpgradeCost(o.Level),
		WorkerQuality:   quality,
	}
	if currentValue > 0 {
		st.Efficiency = net / currentValue
	}
	if net > 0 {
		st.PaybackHours = currentValue / net
	}
	if def.ResearchPerHour > 0 {
		rp := def.ResearchPerHour * math.Pow(def.IncomeMult, float64(o.Level-1))
		if rpBooster {
			rp *= 2
		}
		st.RPHour = rp
	}
	return st
}

// r2 — округление до центов для DTO.
func r2(v float64) float64 { return math.Round(v*100) / 100 }

// ============================================================
// ЖУРНАЛ СОБЫТИЙ
// ============================================================

func logEvent(ctx context.Context, pool *pgxpool.Pool, userID int, typ, ref string, amount float64) {
	pool.Exec(ctx, `
INSERT INTO empire_events (user_id, type, ref, amount) VALUES ($1, $2, $3, $4);`,
		userID, typ, ref, amount)
}

// ============================================================
// ТИК — начисление накопленного дохода.
// Вызывается перед КАЖДЫМ чтением состояния и перед каждым
// действием: доход капает на баланс непрерывно, офлайн — до
// лимита OfflineCapHours (его расширяет Security).
// ============================================================

func Tick(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	now := time.Now()
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	objects, err := LoadObjects(ctx, pool, userID)
	if err != nil {
		return err
	}
	research, err := LoadResearch(ctx, pool, userID)
	if err != nil {
		return err
	}
	boosters, err := LoadBoosters(ctx, pool, userID)
	if err != nil {
		return err
	}
	bonuses := ComputeBonuses(profile, research, boosters, now)

	hours := now.Sub(profile.LastTick).Hours()
	if hours < 0 {
		hours = 0
	}
	if hours > bonuses.OfflineCapHours {
		hours = bonuses.OfflineCapHours
	}

	if hours > 0 {
		rpBooster := boosterActive(boosters, "research_x2", now)
		var totalGross, totalExpenses, rpGain, energyIncome float64
		for i := range objects {
			o := &objects[i]
			def, ok := EmpireObjectByID[o.DefID]
			if !ok {
				continue
			}
			st := ComputeObjectStats(def, *o, bonuses, rpBooster)
			gross := st.IncomePerHour * hours
			exp := st.ExpensesPerHour * hours
			o.LifetimeEarned += gross - exp
			o.WorkerXP += hours
			// satisfaction дрейфует к целевой
			target := 50.0
			if st.IncomePerHour > 0 {
				target = 100 - st.ExpensesPerHour/st.IncomePerHour*100
			}
			target = math.Max(WorkerSatisfactionMin, math.Min(100, target))
			o.Satisfaction += (target - o.Satisfaction) * WorkerSatisfactionDrift * hours
			if o.Satisfaction < WorkerSatisfactionMin {
				o.Satisfaction = WorkerSatisfactionMin
			}
			if o.Satisfaction > 100 {
				o.Satisfaction = 100
			}
			_, err = pool.Exec(ctx, `
UPDATE empire_objects
SET lifetime_earned = $1, worker_xp = $2, satisfaction = $3
WHERE id = $4;`, o.LifetimeEarned, o.WorkerXP, o.Satisfaction, o.ID)
			if err != nil {
				return err
			}
			totalGross += gross
			totalExpenses += exp
			rpGain += st.RPHour * hours
			if def.Sector == "energy" {
				energyIncome += gross
			}
		}

		dividends, err := payDividends(ctx, pool, userID, hours, bonuses.DividendsMult)
		if err != nil {
			return err
		}

		netTotal := totalGross - totalExpenses + dividends
		if netTotal != 0 {
			if err := wallet_sql.AdjustBalance(ctx, pool, userID, netTotal); err != nil {
				return err
			}
		}
		if totalGross+dividends > 0 {
			_, err = pool.Exec(ctx, `
INSERT INTO empire_daily (user_id, day, income, expenses, net)
VALUES ($1, CURRENT_DATE, $2, $3, $4)
ON CONFLICT (user_id, day) DO UPDATE SET
    income = empire_daily.income + EXCLUDED.income,
    expenses = empire_daily.expenses + EXCLUDED.expenses,
    net = empire_daily.net + EXCLUDED.net;`,
				userID, totalGross+dividends, totalExpenses, netTotal)
			if err != nil {
				return err
			}
		}

		// Прогресс контрактов от дохода.
		_, err = pool.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + $2
WHERE user_id = $1 AND status = 'active' AND metric = 'revenue';`,
			userID, totalGross+dividends)
		if err != nil {
			return err
		}
		if energyIncome > 0 {
			_, err = pool.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + $2
WHERE user_id = $1 AND status = 'active' AND metric = 'energy_income';`,
				userID, energyIncome)
			if err != nil {
				return err
			}
		}

		profile.TotalEarned += totalGross + dividends
		profile.AllTimeEarned += totalGross + dividends
		profile.RP += rpGain
		applyXp(profile, (totalGross+dividends)*XpPerIncome)
	}

	// Протухшие контракты.
	_, err = pool.Exec(ctx, `
UPDATE empire_contracts SET status = 'expired'
WHERE user_id = $1 AND status = 'active' AND expires_at < $2;`, userID, now)
	if err != nil {
		return err
	}

	if err := tickAssetPrices(ctx, pool, now); err != nil {
		return err
	}
	if err := ensureContractOffers(ctx, pool, userID, profile.Level); err != nil {
		return err
	}

	profile.LastTick = now
	return saveProfile(ctx, pool, profile)
}

// payDividends начисляет дивиденды по всем holding'ам, сам баланс
// корректирует вызывающий код. Возвращает сумму дивидендов.
func payDividends(ctx context.Context, pool *pgxpool.Pool, userID int, hours, mult float64) (float64, error) {
	rows, err := pool.Query(ctx, `
SELECT h.asset_id, h.shares, h.avg_price, p.price
FROM empire_asset_holdings h
JOIN empire_asset_prices p ON p.asset_id = h.asset_id
WHERE h.user_id = $1 AND h.shares > 0;`, userID)
	if err != nil {
		return 0, err
	}
	type row struct {
		asset  string
		shares int
		price  float64
	}
	list := []row{}
	for rows.Next() {
		var r row
		var avg float64
		if err := rows.Scan(&r.asset, &r.shares, &avg, &r.price); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, r)
	}
	rows.Close()
	total := 0.0
	for _, r := range list {
		def, ok := EmpireAssetByID[r.asset]
		if !ok {
			continue
		}
		d := float64(r.shares) * r.price * def.DailyDividendYield / 24 * hours * mult
		total += d
		_, err = pool.Exec(ctx, `
UPDATE empire_asset_holdings
SET dividends_earned = dividends_earned + $2
WHERE user_id = $1 AND asset_id = $3;`, userID, d, r.asset)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

// tickAssetPrices догоняет random walk цен активов.
func tickAssetPrices(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	for _, def := range EmpireAssets {
		var price float64
		var updatedAt time.Time
		err := pool.QueryRow(ctx, `
SELECT price, updated_at FROM empire_asset_prices WHERE asset_id = $1;`,
			def.ID).Scan(&price, &updatedAt)
		if err != nil {
			continue // не засеяно — SeedEmpireAssets должна была
		}
		steps := int(math.Floor(now.Sub(updatedAt).Minutes() / AssetPriceTickMinutes))
		if steps <= 0 {
			continue
		}
		if steps > AssetMaxCatchupSteps {
			steps = AssetMaxCatchupSteps
		}
		for i := 0; i < steps; i++ {
			price += price * (rand.Float64()*2 - 1) * def.Volatility
			if price < AssetMinPrice {
				price = AssetMinPrice
			}
		}
		price = r2(price)
		// Финальную цену пишем в историю один раз за вызов —
		// точек на графике меньше, но их всегда <= AssetHistoryLimit.
		if _, err := pool.Exec(ctx, `
UPDATE empire_asset_prices SET price = $2, updated_at = $3 WHERE asset_id = $1;`,
			def.ID, price, now); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
INSERT INTO empire_asset_history (asset_id, price) VALUES ($1, $2);`,
			def.ID, price); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
DELETE FROM empire_asset_history
WHERE asset_id = $1
  AND id NOT IN (
      SELECT id FROM empire_asset_history
      WHERE asset_id = $1 ORDER BY id DESC LIMIT $2
  );`, def.ID, AssetHistoryLimit); err != nil {
			return err
		}
	}
	return nil
}

// ensureContractOffers держит количество активных офферов
// на уровне EmpireMaxOffers.
func ensureContractOffers(ctx context.Context, pool *pgxpool.Pool, userID, level int) error {
	var activeCount int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*) FROM empire_contracts
WHERE user_id = $1 AND status = 'active';`, userID).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount >= EmpireMaxOffers {
		return nil
	}
	var lastGen time.Time
	_ = pool.QueryRow(ctx, `
SELECT MAX(created_at) FROM empire_contracts WHERE user_id = $1;`, userID).Scan(&lastGen)
	// Первый заход — сразу выдаём полный набор.
	if activeCount == 0 || lastGen.IsZero() {
		for activeCount < EmpireMaxOffers {
			if err := generateContract(ctx, pool, userID, level); err != nil {
				return err
			}
			activeCount++
		}
		return nil
	}
	if time.Since(lastGen) >= time.Duration(EmpireContractRegenHours*float64(time.Hour)) {
		return generateContract(ctx, pool, userID, level)
	}
	return nil
}

func generateContract(ctx context.Context, pool *pgxpool.Pool, userID, level int) error {
	t := EmpireContractTemplates[rand.Intn(len(EmpireContractTemplates))]
	target := math.Round(t.BaseTarget + t.TargetPerLevel*float64(level-1))
	if target < 1 {
		target = 1
	}
	money := t.MoneyBase + t.MoneyPerLevel*float64(level-1)
	xp := t.XpBase + t.XpPerLevel*float64(level-1)
	rp := t.RpBase + t.RpPerLevel*float64(level-1)
	expires := time.Now().Add(time.Duration(t.DurationHours * float64(time.Hour)))
	_, err := pool.Exec(ctx, `
INSERT INTO empire_contracts
    (user_id, template_id, title, metric, target,
     reward_money, reward_xp, reward_rp, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
		userID, t.ID, t.Name, t.Metric, target, money, xp, rp, expires)
	return err
}

// ============================================================
// СБОРКА STATE
// ============================================================

func buildConfigDTO() map[string]interface{} {
	objDefs := []map[string]interface{}{}
	for _, d := range EmpireObjects {
		objDefs = append(objDefs, map[string]interface{}{
			"id": d.ID, "name": d.Name, "sector": d.Sector, "rarity": d.Rarity,
			"description": d.Description,
			"base_cost":   r2(d.BaseCost), "base_income": r2(d.BaseIncome),
			"base_expenses": r2(d.BaseExpenses), "worker_slots": d.WorkerSlots,
			"rp_per_hour": r2(d.ResearchPerHour), "unlock_level": d.UnlockLevel,
			"upgrade_cost_base": r2(d.UpgradeCostBase), "upgrade_cost_mult": d.UpgradeCostMult,
			"income_mult": d.IncomeMult, "expense_mult": d.ExpenseMult,
			"value_mult": d.ValueMult, "max_level": GlobalMaxLevel,
		})
	}
	researchDefs := []map[string]interface{}{}
	for _, d := range EmpireResearch {
		researchDefs = append(researchDefs, map[string]interface{}{
			"id": d.ID, "name": d.Name, "description": d.Description,
			"effect_kind": d.EffectKind, "effect_per_level": d.EffectPerLevel,
			"max_level":  d.MaxLevel,
			"money_base": d.MoneyBase, "money_mult": d.MoneyMult,
			"rp_base": d.RPBase, "rp_mult": d.RPMult,
		})
	}
	assetDefs := []map[string]interface{}{}
	for _, d := range EmpireAssets {
		assetDefs = append(assetDefs, map[string]interface{}{
			"id": d.ID, "name": d.Name, "description": d.Description,
			"base_price": d.BasePrice, "volatility": d.Volatility,
			"daily_dividend_yield": d.DailyDividendYield,
		})
	}
	boosterDefs := []map[string]interface{}{}
	for _, d := range EmpireBoosters {
		boosterDefs = append(boosterDefs, map[string]interface{}{
			"id": d.ID, "name": d.Name, "description": d.Description,
		})
	}
	contractDefs := []map[string]interface{}{}
	for _, t := range EmpireContractTemplates {
		contractDefs = append(contractDefs, map[string]interface{}{
			"id": t.ID, "name": t.Name, "description": t.Description,
			"metric": t.Metric, "duration_hours": t.DurationHours,
		})
	}
	rarities := []map[string]interface{}{}
	for _, id := range []string{"common", "uncommon", "rare", "epic", "legendary"} {
		rarities = append(rarities, map[string]interface{}{
			"id": id, "order": EmpireRarityOrder[id],
		})
	}
	constants := map[string]interface{}{
		"max_level":                      GlobalMaxLevel,
		"sell_back_rate":                 SellBackRate,
		"worker_hire_cost_rate":          WorkerHireCostRate,
		"worker_income_bonus_per_worker": WorkerIncomeBonusPerWorker,
		"offline_cap_hours":              OfflineCapHours,
		"prestige_min_level":             PrestigeMinLevel,
		"prestige_income_bonus_per_pp":   PrestigeIncomeBonusPerPP,
		"level_income_bonus_per_level":   LevelIncomeBonusPerLevel,
		"asset_trade_fee_rate":           AssetTradeFeeRate,
		"asset_price_tick_minutes":       AssetPriceTickMinutes,
		"empire_max_offers":              EmpireMaxOffers,
		"contract_regen_hours":           EmpireContractRegenHours,
	}
	return map[string]interface{}{
		"sectors":            EmpireSectors,
		"rarities":           rarities,
		"objects":            objDefs,
		"research":           researchDefs,
		"assets":             assetDefs,
		"boosters":           boosterDefs,
		"booster_durations":  BoosterDurations,
		"booster_prices":     BoosterPrices,
		"contract_templates": contractDefs,
		"constants":          constants,
	}
}

func CalcPrestigePP(p *EmpireProfile) int {
	fromEarned := int(math.Floor(math.Sqrt(p.TotalEarned / PrestigePPDivisor)))
	fromLevel := (p.Level - PrestigeMinLevel) / PrestigePPPerLevelEvery
	if fromLevel < 0 {
		fromLevel = 0
	}
	return PrestigeBasePP + fromEarned + fromLevel
}

func BuildState(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]interface{}, error) {
	if err := Tick(ctx, pool, userID); err != nil {
		return nil, err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	objects, err := LoadObjects(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	research, err := LoadResearch(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	boosters, err := LoadBoosters(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	holdings, err := LoadHoldings(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	prices, err := loadAssetPrices(ctx, pool)
	if err != nil {
		return nil, err
	}
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	bonuses := ComputeBonuses(profile, research, boosters, time.Now())
	rpBooster := boosterActive(boosters, "research_x2", time.Now())
	now := time.Now()

	perSector := map[string]map[string]interface{}{}
	for _, s := range EmpireSectors {
		perSector[s.ID] = map[string]interface{}{
			"count": 0, "income_per_hour": 0.0, "value": 0.0,
		}
	}

	incomeH, expensesH, value, rpH, workerCount := 0.0, 0.0, 0.0, 0.0, 0
	objsDTO := []map[string]interface{}{}
	leaderboard := []map[string]interface{}{}
	for _, o := range objects {
		def, ok := EmpireObjectByID[o.DefID]
		if !ok {
			continue
		}
		st := ComputeObjectStats(def, o, bonuses, rpBooster)
		name := def.Name
		if o.CustomName != "" {
			name = o.CustomName
		}
		ageH := r2(now.Sub(o.PurchasedAt).Hours())
		incomeH += st.IncomePerHour
		expensesH += st.ExpensesPerHour
		value += st.CurrentValue
		rpH += st.RPHour
		workerCount += o.WorkersHired

		sec := perSector[def.Sector]
		sec["count"] = sec["count"].(int) + 1
		sec["income_per_hour"] = sec["income_per_hour"].(float64) + st.IncomePerHour
		sec["value"] = sec["value"].(float64) + st.CurrentValue

		upgradeGain := st.IncomePerHour * (def.IncomeMult - 1)
		objsDTO = append(objsDTO, map[string]interface{}{
			"instance_id":         o.ID,
			"def_id":              def.ID,
			"name":                name,
			"sector":              def.Sector,
			"rarity":              def.Rarity,
			"level":               o.Level,
			"max_level":           GlobalMaxLevel,
			"age_hours":           ageH,
			"workers_hired":       o.WorkersHired,
			"worker_slots":        def.WorkerSlots,
			"worker_quality":      r2(st.WorkerQuality),
			"satisfaction":        r2(o.Satisfaction),
			"income_per_hour":     r2(st.IncomePerHour),
			"expenses_per_hour":   r2(st.ExpensesPerHour),
			"net_per_hour":        r2(st.NetPerHour),
			"current_value":       r2(st.CurrentValue),
			"sell_value":          r2(st.SellValue),
			"invested":            r2(o.Invested),
			"lifetime_earned":     r2(o.LifetimeEarned),
			"efficiency":          r2(st.Efficiency * 100),
			"payback_hours":       r2(st.PaybackHours),
			"upgrade_cost":        r2(st.UpgradeCost),
			"upgrade_income_gain": r2(upgradeGain),
			"rp_per_hour":         r2(st.RPHour),
			"purchased_at":        o.PurchasedAt.Unix(),
		})

		// Метрики лидерборда объектов.
		roi := 0.0
		if o.Invested > 0 {
			roi = (o.LifetimeEarned - o.Invested) / o.Invested
		}
		upgInv := math.Max(0, o.Invested-def.BaseCost)
		upgEff := 0.0
		if upgInv > 0 {
			upgEff = (def.IncomeAt(o.Level) - def.IncomeAt(1)) / upgInv
		}
		leaderboard = append(leaderboard, map[string]interface{}{
			"instance_id":        o.ID,
			"name":               name,
			"def_id":             def.ID,
			"sector":             def.Sector,
			"roi":                r2(roi * 100),
			"total_profit":       r2(o.LifetimeEarned),
			"profit_per_hour":    r2(st.NetPerHour),
			"efficiency":         r2(st.Efficiency * 100),
			"upgrade_efficiency": r2(upgEff * 100),
			"payback_hours":      r2(st.PaybackHours),
		})
	}

	// Акции.
	holdingsValue := 0.0
	holdingsDTO := map[string]interface{}{}
	for assetID, h := range holdings {
		price := prices[assetID]
		v := float64(h.Shares) * price
		holdingsValue += v
		holdingsDTO[assetID] = map[string]interface{}{
			"shares":           h.Shares,
			"avg_price":        r2(h.AvgPrice),
			"price":            r2(price),
			"value":            r2(v),
			"dividends_earned": r2(h.DividendsEarned),
		}
	}
	pricesDTO := map[string]interface{}{}
	historyDTO := map[string][]map[string]interface{}{}
	for _, def := range EmpireAssets {
		pricesDTO[def.ID] = r2(prices[def.ID])
		rows, err := pool.Query(ctx, `
SELECT price, created_at FROM empire_asset_history
WHERE asset_id = $1 ORDER BY id DESC LIMIT $2;`, def.ID, AssetHistoryLimit)
		if err != nil {
			return nil, err
		}
		hist := []map[string]interface{}{}
		for rows.Next() {
			var p float64
			var ts time.Time
			if err := rows.Scan(&p, &ts); err != nil {
				rows.Close()
				return nil, err
			}
			hist = append(hist, map[string]interface{}{
				"price": r2(p), "t": ts.Unix(),
			})
		}
		rows.Close()
		// Переворачиваем — график идёт от старых к новым.
		for i, j := 0, len(hist)-1; i < j; i, j = i+1, j-1 {
			hist[i], hist[j] = hist[j], hist[i]
		}
		historyDTO[def.ID] = hist
	}

	// Исследования с ценами следующего уровня.
	researchDTO := map[string]int{}
	for k, v := range research {
		researchDTO[k] = v
	}
	researchDefsDTO := []map[string]interface{}{}
	for _, d := range EmpireResearch {
		cur := research[d.ID]
		entry := map[string]interface{}{
			"id": d.ID, "name": d.Name, "description": d.Description,
			"effect_kind": d.EffectKind, "effect_per_level": d.EffectPerLevel,
			"max_level": d.MaxLevel, "level": cur,
		}
		if cur < d.MaxLevel {
			entry["next_money"] = r2(d.MoneyCost(cur))
			entry["next_rp"] = d.RPCost(cur)
		}
		researchDefsDTO = append(researchDefsDTO, entry)
	}

	// Контракты (только активные, не протухшие).
	contractsDTO := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT id, template_id, title, metric, target, progress,
       reward_money, reward_xp, reward_rp, expires_at
FROM empire_contracts
WHERE user_id = $1 AND status = 'active' AND expires_at > now()
ORDER BY id;`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		tmplDesc := map[string]string{}
		for _, t := range EmpireContractTemplates {
			tmplDesc[t.ID] = t.Description
		}
		for rows.Next() {
			var id int
			var tmpl, title, metric string
			var target, progress, rm, rx, rr float64
			var exp time.Time
			if err := rows.Scan(&id, &tmpl, &title, &metric, &target, &progress,
				&rm, &rx, &rr, &exp); err != nil {
				return nil, err
			}
			contractsDTO = append(contractsDTO, map[string]interface{}{
				"id": id, "template_id": tmpl, "title": title,
				"description": tmplDesc[tmpl], "metric": metric,
				"target": r2(target), "progress": r2(progress),
				"reward_money": r2(rm), "reward_xp": r2(rx), "reward_rp": r2(rr),
				"expires_at": exp.Unix(),
			})
		}
	}

	// Бустеры.
	boostersDTO := map[string]interface{}{}
	for typ, exp := range boosters {
		boostersDTO[typ] = exp.Unix()
	}

	// Аналитика: дневная статистика + последние события.
	dailyDTO := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT day, income, expenses, net FROM empire_daily
WHERE user_id = $1 ORDER BY day DESC LIMIT $2;`, userID, DailyStatsLimit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var day time.Time
			var inc, exp, net float64
			if err := rows.Scan(&day, &inc, &exp, &net); err != nil {
				rows.Close()
				return nil, err
			}
			dailyDTO = append(dailyDTO, map[string]interface{}{
				"day":    day.Format("2006-01-02"),
				"income": r2(inc), "expenses": r2(exp), "net": r2(net),
			})
		}
		rows.Close()
		for i, j := 0, len(dailyDTO)-1; i < j; i, j = i+1, j-1 {
			dailyDTO[i], dailyDTO[j] = dailyDTO[j], dailyDTO[i]
		}
	}
	eventsDTO := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT type, ref, amount, created_at FROM empire_events
WHERE user_id = $1 ORDER BY id DESC LIMIT 50;`, userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var typ, ref string
			var amount float64
			var ts time.Time
			if err := rows.Scan(&typ, &ref, &amount, &ts); err != nil {
				return nil, err
			}
			eventsDTO = append(eventsDTO, map[string]interface{}{
				"type": typ, "ref": ref, "amount": r2(amount), "t": ts.Unix(),
			})
		}
	}

	ppGain := CalcPrestigePP(profile)

	return map[string]interface{}{
		"profile": map[string]interface{}{
			"level":           profile.Level,
			"xp":              r2(profile.XP),
			"xp_for_next":     EmpireXpForLevel(profile.Level),
			"rp":              r2(profile.RP),
			"total_earned":    r2(profile.TotalEarned),
			"total_spent":     r2(profile.TotalSpent),
			"all_time_earned": r2(profile.AllTimeEarned),
			"prestige_count":  profile.PrestigeCount,
			"prestige_points": profile.PrestigePoints,
			"last_tick":       profile.LastTick.Unix(),
		},
		"balance":      r2(balance),
		"empire_value": r2(value + holdingsValue),
		"assets_value": r2(holdingsValue),
		"object_count": len(objsDTO),
		"worker_count": workerCount,
		"rates": map[string]interface{}{
			"income_per_hour":   r2(incomeH),
			"expenses_per_hour": r2(expensesH),
			"net_per_hour":      r2(incomeH - expensesH),
			"income_per_day":    r2(incomeH * 24),
			"net_per_day":       r2((incomeH - expensesH) * 24),
			"rp_per_hour":       r2(rpH),
		},
		"objects":       objsDTO,
		"research":      researchDTO,
		"research_defs": researchDefsDTO,
		"assets": map[string]interface{}{
			"prices":   pricesDTO,
			"holdings": holdingsDTO,
			"history":  historyDTO,
		},
		"contracts": contractsDTO,
		"boosters":  boostersDTO,
		"analytics": map[string]interface{}{
			"total_earned":    r2(profile.TotalEarned),
			"total_spent":     r2(profile.TotalSpent),
			"all_time_earned": r2(profile.AllTimeEarned),
			"per_sector":      perSector,
			"daily":           dailyDTO,
			"recent_events":   eventsDTO,
		},
		"leaderboard": leaderboard,
		"prestige": map[string]interface{}{
			"available":            profile.Level >= PrestigeMinLevel,
			"min_level":            PrestigeMinLevel,
			"pp_gain":              ppGain,
			"prestige_points":      profile.PrestigePoints,
			"prestige_count":       profile.PrestigeCount,
			"income_bonus_per_pp":  PrestigeIncomeBonusPerPP,
			"current_income_bonus": PrestigeIncomeBonusPerPP * float64(profile.PrestigePoints),
		},
		"config":      buildConfigDTO(),
		"server_time": now.Unix(),
	}, nil
}
