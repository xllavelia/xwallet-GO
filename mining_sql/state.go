package mining_sql

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ===================== ВНУТРЕННИЕ МОДЕЛИ =====================

type profileRow struct {
	Energy             float64
	MaxEnergy          float64
	ExtraSlots         int
	LifetimeEarned     float64
	LifetimeSpent      float64
	LifetimeEnergyUsed float64
	LastTickAt         time.Time
}

type serverRow struct {
	ID              int64
	CatalogID       string
	Rank            int
	AwakeUntil      *time.Time
	Exhausted       bool
	TotalEarned     float64
	TotalEnergyUsed float64
	PurchasedAt     time.Time
}

// activeBuffs — какие баффы сейчас активны у игрока (уже проверено expires_at > now()).
type activeBuffs struct {
	NoSleep       bool
	ProfitBoost   bool
	PowerBoost    bool
	PowerBoostEff bool
}

// ===================== ПОЛУЧЕНИЕ / СОЗДАНИЕ ПРОФИЛЯ =====================

func getOrCreateProfile(ctx context.Context, tx pgx.Tx, userID int) (profileRow, error) {
	_, err := tx.Exec(ctx, `
		INSERT INTO mining_profile (user_id, energy, max_energy, last_tick_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id) DO NOTHING;
	`, userID, DefaultStartEnergy, DefaultMaxEnergy)
	if err != nil {
		return profileRow{}, err
	}
	var p profileRow
	err = tx.QueryRow(ctx, `
		SELECT energy, max_energy, extra_slots, lifetime_earned, lifetime_spent, lifetime_energy_used, last_tick_at
		FROM mining_profile WHERE user_id = $1;
	`, userID).Scan(&p.Energy, &p.MaxEnergy, &p.ExtraSlots, &p.LifetimeEarned, &p.LifetimeSpent, &p.LifetimeEnergyUsed, &p.LastTickAt)
	return p, err
}

func loadActiveBuffs(ctx context.Context, tx pgx.Tx, userID int) (activeBuffs, error) {
	rows, err := tx.Query(ctx, `
		SELECT buff_type FROM mining_buffs WHERE user_id = $1 AND expires_at > now();
	`, userID)
	if err != nil {
		return activeBuffs{}, err
	}
	defer rows.Close()
	var b activeBuffs
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return activeBuffs{}, err
		}
		switch t {
		case BuffNoSleep:
			b.NoSleep = true
		case BuffProfitBoost:
			b.ProfitBoost = true
		case BuffPowerBoost:
			b.PowerBoost = true
		case BuffPowerBoostEf:
			b.PowerBoostEff = true
		}
	}
	return b, rows.Err()
}

// ===================== ЭФФЕКТИВНЫЕ ХАРАКТЕРИСТИКИ =====================

type effectiveStats struct {
	Power         float64
	ProfitPerHour float64
	EnergyPerHour float64
}

func computeEffectiveStats(def ServerDef, rank int, buffs activeBuffs) effectiveStats {
	tier := RankTiers[rank-1]
	profitMult := tier.ProfitMult
	energyMult := tier.EnergyMult
	powerMult := tier.PowerMult

	for _, key := range def.Perks {
		if perk, ok := PerkDefs[key]; ok {
			profitMult *= perk.ProfitMult
			energyMult *= perk.EnergyMult
		}
	}

	if buffs.ProfitBoost {
		profitMult *= 2
	}
	if buffs.PowerBoost {
		profitMult *= 2
		powerMult *= 2
		energyMult *= 2
	}
	if buffs.PowerBoostEff {
		profitMult *= 2
		powerMult *= 2
		energyMult *= 0.5
	}

	return effectiveStats{
		Power:         def.Power * powerMult,
		ProfitPerHour: def.ProfitPerHour * profitMult,
		EnergyPerHour: def.EnergyPerHour * energyMult,
	}
}

// isAwake — спит сервер или работает, с учётом Always On / максимального ранга / баффа no_sleep.
func isAwake(def ServerDef, s serverRow, buffs activeBuffs, now time.Time) bool {
	if s.Exhausted {
		return false
	}
	if def.SleepMinutes == 0 { // легендарки — Always On с самого начала
		return true
	}
	if s.Rank == 10 && def.PermanentNoSleepAtMaxRank {
		return true
	}
	if buffs.NoSleep {
		return true
	}
	return s.AwakeUntil != nil && s.AwakeUntil.After(now)
}

// ===================== ТИК =====================

// Tick пересчитывает доход/энергию с момента last_tick_at до now() и обновляет БД.
// Вызывается перед каждым чтением состояния и перед каждым действием.
func Tick(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	profile, err := getOrCreateProfile(ctx, tx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	dt := now.Sub(profile.LastTickAt)
	if dt <= 0 {
		return tx.Commit(ctx)
	}
	if dt > MaxCatchUpDuration {
		dt = MaxCatchUpDuration
	}
	dtHours := dt.Hours()

	buffs, err := loadActiveBuffs(ctx, tx, userID)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, catalog_id, rank, awake_until, exhausted, total_earned, total_energy_used, purchased_at
		FROM mining_servers WHERE user_id = $1 FOR UPDATE;
	`, userID)
	if err != nil {
		return err
	}
	var servers []serverRow
	for rows.Next() {
		var s serverRow
		if err := rows.Scan(&s.ID, &s.CatalogID, &s.Rank, &s.AwakeUntil, &s.Exhausted, &s.TotalEarned, &s.TotalEnergyUsed, &s.PurchasedAt); err != nil {
			rows.Close()
			return err
		}
		servers = append(servers, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	type active struct {
		row          serverRow
		def          ServerDef
		energyPerSec float64
		profitPerSec float64
		capRemaining float64
	}

	var activeList []active
	var totalEnergyDemand float64
	for _, s := range servers {
		def, ok := GetServerDef(s.CatalogID)
		if !ok {
			continue
		}
		if !isAwake(def, s, buffs, now) {
			continue
		}
		eff := computeEffectiveStats(def, s.Rank, buffs)
		capRemaining := def.LifetimeProfitCap - s.TotalEarned
		if capRemaining <= 0 {
			continue
		}
		energyPerSec := eff.EnergyPerHour / 3600
		profitPerSec := eff.ProfitPerHour / 3600
		activeList = append(activeList, active{row: s, def: def, energyPerSec: energyPerSec, profitPerSec: profitPerSec, capRemaining: capRemaining})
		totalEnergyDemand += energyPerSec * dt.Seconds()
	}

	ratio := 1.0
	if totalEnergyDemand > profile.Energy && totalEnergyDemand > 0 {
		ratio = profile.Energy / totalEnergyDemand
	}

	var totalEarned, totalEnergyUsed float64
	for _, a := range activeList {
		usedEnergy := a.energyPerSec * dt.Seconds() * ratio
		earned := a.profitPerSec * dt.Seconds() * ratio
		if earned > a.capRemaining {
			earned = a.capRemaining
		}

		newTotalEarned := a.row.TotalEarned + earned
		exhausted := newTotalEarned >= a.def.LifetimeProfitCap

		_, err = tx.Exec(ctx, `
			UPDATE mining_servers
			SET total_earned = $1, total_energy_used = total_energy_used + $2, exhausted = $3, last_tick_at = now()
			WHERE id = $4;
		`, newTotalEarned, usedEnergy, exhausted, a.row.ID)
		if err != nil {
			return err
		}

		totalEarned += earned
		totalEnergyUsed += usedEnergy
	}

	newEnergy := profile.Energy - totalEnergyUsed
	if newEnergy < 0 {
		newEnergy = 0
	}
	if newEnergy > profile.MaxEnergy {
		newEnergy = profile.MaxEnergy
	}

	_, err = tx.Exec(ctx, `
		UPDATE mining_profile
		SET energy = $1, lifetime_earned = lifetime_earned + $2, lifetime_energy_used = lifetime_energy_used + $3, last_tick_at = now()
		WHERE user_id = $4;
	`, newEnergy, totalEarned, totalEnergyUsed, userID)
	if err != nil {
		return err
	}

	if totalEarned > 0 {
		if err := AdjustWalletBalanceTx(ctx, tx, userID, totalEarned); err != nil {
			return err
		}
	}

	_ = dtHours
	return tx.Commit(ctx)
}

// ===================== DTO =====================

type ServerDTO struct {
	ID              int64    `json:"id"`
	CatalogID       string   `json:"catalogId"`
	Name            string   `json:"name"`
	Country         string   `json:"country"`
	Rarity          string   `json:"rarity"`
	Rank            int      `json:"rank"`
	MaxRank         int      `json:"maxRank"`
	Power           float64  `json:"power"`
	ProfitPerHour   float64  `json:"profitPerHour"`
	ProfitPerDay    float64  `json:"profitPerDay"`
	EnergyPerHour   float64  `json:"energyPerHour"`
	Awake           bool     `json:"awake"`
	AwakeUntil      *string  `json:"awakeUntil,omitempty"`
	SleepMinutes    int      `json:"sleepMinutes"`
	AlwaysOn        bool     `json:"alwaysOn"`
	Exhausted       bool     `json:"exhausted"`
	TotalEarned     float64  `json:"totalEarned"`
	LifetimeCap     float64  `json:"lifetimeCap"`
	TotalEnergyUsed float64  `json:"totalEnergyUsed"`
	PurchasedAt     string   `json:"purchasedAt"`
	Perks           []string `json:"perks"`
	NextRankCost    float64  `json:"nextRankCost,omitempty"`
	CanUpgrade      bool     `json:"canUpgrade"`
}

type BuffDTO struct {
	Type             string `json:"type"`
	SecondsRemaining int64  `json:"secondsRemaining"`
}

type ProfileDTO struct {
	Energy    float64 `json:"energy"`
	MaxEnergy float64 `json:"maxEnergy"`

	LifetimeEarned     float64 `json:"lifetimeEarned"`
	LifetimeSpent      float64 `json:"lifetimeSpent"`
	LifetimeEnergyUsed float64 `json:"lifetimeEnergyUsed"`
}

type StateResponse struct {
	Profile ProfileDTO  `json:"profile"`
	Servers []ServerDTO `json:"servers"`
	Buffs   []BuffDTO   `json:"buffs"`

	TotalPower           float64 `json:"totalPower"`
	TotalProfitPerHour   float64 `json:"totalProfitPerHour"`
	TotalProfitPerDay    float64 `json:"totalProfitPerDay"`
	TotalEnergyPerHour   float64 `json:"totalEnergyPerHour"`
	EstimatedMinutesLeft float64 `json:"estimatedMinutesLeft"`

	ShopServers []ShopServerDTO `json:"shopServers"`
	ShopItems   ShopItemsDTO    `json:"shopItems"`

	TotalOwnedServers int `json:"totalOwnedServers"`
	MaxTotalServers   int `json:"maxTotalServers"`
}

type ShopServerDTO struct {
	CatalogID     string   `json:"catalogId"`
	Name          string   `json:"name"`
	Country       string   `json:"country"`
	Rarity        string   `json:"rarity"`
	Price         float64  `json:"price"`
	Power         float64  `json:"power"`
	ProfitPerHour float64  `json:"profitPerHour"`
	ProfitPerDay  float64  `json:"profitPerDay"`
	EnergyPerHour float64  `json:"energyPerHour"`
	SleepMinutes  int      `json:"sleepMinutes"`
	AlwaysOn      bool     `json:"alwaysOn"`
	LifetimeCap   float64  `json:"lifetimeCap"`
	Perks         []string `json:"perks"`
	OwnedCount    int      `json:"ownedCount"`
	MaxCopies     int      `json:"maxCopies"`
	CanBuy        bool     `json:"canBuy"`
}

type ShopItemsDTO struct {
	Buffs             map[string][]BuffDuration `json:"buffs"`
	EnergyPacks       []EnergyPackDef           `json:"energyPacks"`
	MaxEnergyExpander EnergyPackDef             `json:"maxEnergyExpander"`
	SlotExpander      EnergyPackDef             `json:"slotExpander"`
}

// ===================== ПОСТРОЕНИЕ ОТВЕТА =====================

func GetState(ctx context.Context, pool *pgxpool.Pool, userID int) (StateResponse, error) {
	if err := Tick(ctx, pool, userID); err != nil {
		return StateResponse{}, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return StateResponse{}, err
	}
	defer tx.Rollback(ctx)

	profile, err := getOrCreateProfile(ctx, tx, userID)
	if err != nil {
		return StateResponse{}, err
	}
	buffs, err := loadActiveBuffs(ctx, tx, userID)
	if err != nil {
		return StateResponse{}, err
	}

	buffExpiries := map[string]time.Time{}
	rows, err := tx.Query(ctx, `SELECT buff_type, expires_at FROM mining_buffs WHERE user_id = $1 AND expires_at > now();`, userID)
	if err != nil {
		return StateResponse{}, err
	}
	for rows.Next() {
		var t string
		var exp time.Time
		if err := rows.Scan(&t, &exp); err != nil {
			rows.Close()
			return StateResponse{}, err
		}
		buffExpiries[t] = exp
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return StateResponse{}, err
	}

	now := time.Now()
	var buffDTOs []BuffDTO
	for t, exp := range buffExpiries {
		buffDTOs = append(buffDTOs, BuffDTO{Type: t, SecondsRemaining: int64(exp.Sub(now).Seconds())})
	}

	srows, err := tx.Query(ctx, `
		SELECT id, catalog_id, rank, awake_until, exhausted, total_earned, total_energy_used, purchased_at
		FROM mining_servers WHERE user_id = $1 ORDER BY purchased_at ASC;
	`, userID)
	if err != nil {
		return StateResponse{}, err
	}
	var serverRows []serverRow
	for srows.Next() {
		var s serverRow
		if err := srows.Scan(&s.ID, &s.CatalogID, &s.Rank, &s.AwakeUntil, &s.Exhausted, &s.TotalEarned, &s.TotalEnergyUsed, &s.PurchasedAt); err != nil {
			srows.Close()
			return StateResponse{}, err
		}
		serverRows = append(serverRows, s)
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return StateResponse{}, err
	}

	ownedCount := map[string]int{}
	var serverDTOs []ServerDTO
	var totalPower, totalProfitHr, totalEnergyHr float64

	for _, s := range serverRows {
		def, ok := GetServerDef(s.CatalogID)
		if !ok {
			continue
		}
		ownedCount[s.CatalogID]++
		eff := computeEffectiveStats(def, s.Rank, buffs)
		awake := isAwake(def, s, buffs, now)
		if awake {
			totalPower += eff.Power
			totalProfitHr += eff.ProfitPerHour
			totalEnergyHr += eff.EnergyPerHour
		}

		var awakeUntilStr *string
		if s.AwakeUntil != nil {
			str := s.AwakeUntil.Format(time.RFC3339)
			awakeUntilStr = &str
		}

		canUpgrade := s.Rank < 10
		var nextCost float64
		if canUpgrade {
			nextCost = def.Price * RankTiers[s.Rank].UpgradeCostMult
		}

		serverDTOs = append(serverDTOs, ServerDTO{
			ID: s.ID, CatalogID: s.CatalogID, Name: def.Name, Country: def.Country, Rarity: string(def.Rarity),
			Rank: s.Rank, MaxRank: 10,
			Power: eff.Power, ProfitPerHour: eff.ProfitPerHour, ProfitPerDay: eff.ProfitPerHour * 24,
			EnergyPerHour: eff.EnergyPerHour,
			Awake:         awake, AwakeUntil: awakeUntilStr,
			SleepMinutes: def.SleepMinutes, AlwaysOn: def.SleepMinutes == 0 || (s.Rank == 10 && def.PermanentNoSleepAtMaxRank),
			Exhausted: s.Exhausted, TotalEarned: s.TotalEarned, LifetimeCap: def.LifetimeProfitCap,
			TotalEnergyUsed: s.TotalEnergyUsed, PurchasedAt: s.PurchasedAt.Format(time.RFC3339),
			Perks: def.Perks, NextRankCost: nextCost, CanUpgrade: canUpgrade,
		})
	}

	maxTotal := BaseMaxCopiesPerServer + profile.ExtraSlots
	totalOwned := len(serverRows)
	var shopServers []ShopServerDTO
	for _, def := range ServerCatalog {
		owned := ownedCount[def.ID]
		shopServers = append(shopServers, ShopServerDTO{
			CatalogID: def.ID, Name: def.Name, Country: def.Country, Rarity: string(def.Rarity),
			Price: def.Price, Power: def.Power, ProfitPerHour: def.ProfitPerHour, ProfitPerDay: def.ProfitPerHour * 24,
			EnergyPerHour: def.EnergyPerHour, SleepMinutes: def.SleepMinutes, AlwaysOn: def.SleepMinutes == 0,
			LifetimeCap: def.LifetimeProfitCap, Perks: def.Perks,
			OwnedCount: owned, CanBuy: totalOwned < maxTotal,
		})
	}

	estimatedMinutesLeft := 0.0
	if totalEnergyHr > 0 {
		estimatedMinutesLeft = (profile.Energy / totalEnergyHr) * 60
	}

	if err := tx.Commit(ctx); err != nil {
		return StateResponse{}, err
	}

	return StateResponse{
		Profile: ProfileDTO{
			Energy: profile.Energy, MaxEnergy: profile.MaxEnergy,
			LifetimeEarned: profile.LifetimeEarned, LifetimeSpent: profile.LifetimeSpent, LifetimeEnergyUsed: profile.LifetimeEnergyUsed,
		},
		Servers: serverDTOs, Buffs: buffDTOs,
		TotalPower: totalPower, TotalProfitPerHour: totalProfitHr, TotalProfitPerDay: totalProfitHr * 24,
		TotalEnergyPerHour: totalEnergyHr, EstimatedMinutesLeft: estimatedMinutesLeft,
		ShopServers: shopServers,
		ShopItems: ShopItemsDTO{
			Buffs: BuffCatalog, EnergyPacks: EnergyPacks,
			MaxEnergyExpander: EnergyPackDef{ID: "max_energy", Amount: MaxEnergyExpanderAddAmount, Price: MaxEnergyExpanderPrice},
			SlotExpander:      EnergyPackDef{ID: "slots", Amount: SlotExpanderAddSlots, Price: SlotExpanderPrice},
		},
		TotalOwnedServers: totalOwned, MaxTotalServers: maxTotal,
	}, nil
}
