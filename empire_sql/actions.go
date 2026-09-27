package empire_sql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/wallet_sql"
)

// Все действия сначала дотикивают доход (Tick), чтобы баланс
// и last_tick были актуальны перед проверками.

func ActionBuyObject(ctx context.Context, pool *pgxpool.Pool, userID int, defID string) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	def, ok := EmpireObjectByID[defID]
	if !ok {
		return errors.New("unknown facility")
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	if profile.Level < def.UnlockLevel {
		return fmt.Errorf("requires empire level %d", def.UnlockLevel)
	}
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < def.BaseCost {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -def.BaseCost); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO empire_objects (user_id, def_id, invested)
VALUES ($1, $2, $3);`, userID, def.ID, def.BaseCost); err != nil {
		return err
	}
	profile.TotalSpent += def.BaseCost
	applyXp(profile, def.BaseCost*XpPerSpent)
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "purchase", def.ID, def.BaseCost)
	// Прогресс контрактов на покупку объектов.
	pool.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + 1
WHERE user_id = $1 AND status = 'active' AND metric = 'objects_bought';`, userID)
	if EmpireRarityOrder[def.Rarity] >= EmpireRarityOrder["rare"] {
		pool.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + 1
WHERE user_id = $1 AND status = 'active' AND metric = 'facility_rare';`, userID)
	}
	return nil
}

// Загружает объект и проверяет, что он принадлежит юзеру.
func loadOwnedObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) (*ObjectInstance, *EmpireObjectDef, error) {
	var o ObjectInstance
	err := pool.QueryRow(ctx, `
SELECT id, def_id, COALESCE(custom_name, ''), level, workers_hired,
       worker_xp, satisfaction, invested, lifetime_earned, purchased_at
FROM empire_objects WHERE id = $1 AND user_id = $2;`, objectID, userID).
		Scan(&o.ID, &o.DefID, &o.CustomName, &o.Level, &o.WorkersHired,
			&o.WorkerXP, &o.Satisfaction, &o.Invested, &o.LifetimeEarned, &o.PurchasedAt)
	if err != nil {
		return nil, nil, errors.New("facility not found")
	}
	def, ok := EmpireObjectByID[o.DefID]
	if !ok {
		return nil, nil, errors.New("unknown facility type")
	}
	return &o, &def, nil
}

func ActionUpgradeObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	o, def, err := loadOwnedObject(ctx, pool, userID, objectID)
	if err != nil {
		return err
	}
	if o.Level >= GlobalMaxLevel {
		return errors.New("facility is at max level")
	}
	cost := def.UpgradeCost(o.Level)
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < cost {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -cost); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
UPDATE empire_objects SET level = level + 1, invested = invested + $2
WHERE id = $1;`, objectID, cost); err != nil {
		return err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	profile.TotalSpent += cost
	applyXp(profile, cost*XpPerSpent)
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "upgrade", def.ID, cost)
	return nil
}

func ActionSellObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	o, def, err := loadOwnedObject(ctx, pool, userID, objectID)
	if err != nil {
		return err
	}
	proceeds := o.Invested * SellBackRate
	if _, err := pool.Exec(ctx, `
DELETE FROM empire_objects WHERE id = $1;`, objectID); err != nil {
		return err
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, proceeds); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "sale", def.ID, proceeds)
	return nil
}

func ActionRenameObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return errors.New("name must be 1-64 characters")
	}
	if _, err := pool.Exec(ctx, `
UPDATE empire_objects SET custom_name = $2
WHERE id = $1 AND user_id = $3;`, objectID, name, userID); err != nil {
		return err
	}
	return nil
}

func ActionHireWorker(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	o, def, err := loadOwnedObject(ctx, pool, userID, objectID)
	if err != nil {
		return err
	}
	if o.WorkersHired >= def.WorkerSlots {
		return errors.New("all worker slots are filled")
	}
	cost := def.BaseCost * WorkerHireCostRate
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < cost {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -cost); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
UPDATE empire_objects SET workers_hired = workers_hired + 1
WHERE id = $1;`, objectID); err != nil {
		return err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	profile.TotalSpent += cost
	applyXp(profile, cost*XpPerSpent)
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "hire", def.ID, cost)
	return nil
}

func ActionBuyResearch(ctx context.Context, pool *pgxpool.Pool, userID int, direction string) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	def, ok := EmpireResearchByID[direction]
	if !ok {
		return errors.New("unknown research direction")
	}
	research, err := LoadResearch(ctx, pool, userID)
	if err != nil {
		return err
	}
	cur := research[direction]
	if cur >= def.MaxLevel {
		return errors.New("research is at max level")
	}
	money := def.MoneyCost(cur)
	rp := def.RPCost(cur)
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	if profile.RP < rp {
		return errors.New("not enough research points")
	}
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < money {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -money); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO empire_research (user_id, direction, level)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, direction) DO UPDATE SET level = empire_research.level + 1;`,
		userID, direction); err != nil {
		return err
	}
	profile.RP -= rp
	profile.TotalSpent += money
	applyXp(profile, money*XpPerResearchSpent)
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "research", direction, money)
	return nil
}

func ActionBuyAsset(ctx context.Context, pool *pgxpool.Pool, userID int, assetID string, shares int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	def, ok := EmpireAssetByID[assetID]
	if !ok {
		return errors.New("unknown asset")
	}
	if shares < 1 || shares > 1000000 {
		return errors.New("invalid shares amount")
	}
	var price float64
	if err := pool.QueryRow(ctx, `
SELECT price FROM empire_asset_prices WHERE asset_id = $1;`, assetID).Scan(&price); err != nil {
		return err
	}
	cost := price * float64(shares)
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < cost {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -cost); err != nil {
		return err
	}
	// Пересчёт средней цены покупки.
	if _, err := pool.Exec(ctx, `
INSERT INTO empire_asset_holdings (user_id, asset_id, shares, avg_price)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, asset_id) DO UPDATE SET
    avg_price = (empire_asset_holdings.avg_price * empire_asset_holdings.shares + $4 * $3)
                / (empire_asset_holdings.shares + $3),
    shares = empire_asset_holdings.shares + $3;`,
		userID, assetID, shares, price); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "asset_buy", def.ID, cost)
	return nil
}

func ActionSellAsset(ctx context.Context, pool *pgxpool.Pool, userID int, assetID string, shares int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	def, ok := EmpireAssetByID[assetID]
	if !ok {
		return errors.New("unknown asset")
	}
	if shares < 1 {
		return errors.New("invalid shares amount")
	}
	var owned int
	var avg float64
	err := pool.QueryRow(ctx, `
SELECT shares, avg_price FROM empire_asset_holdings
WHERE user_id = $1 AND asset_id = $2;`, userID, assetID).Scan(&owned, &avg)
	if err != nil || owned < shares {
		return errors.New("not enough shares")
	}
	var price float64
	if err := pool.QueryRow(ctx, `
SELECT price FROM empire_asset_prices WHERE asset_id = $1;`, assetID).Scan(&price); err != nil {
		return err
	}
	proceeds := price * float64(shares) * (1 - AssetTradeFeeRate)
	newShares := owned - shares
	newAvg := avg
	if newShares == 0 {
		newAvg = 0
	}
	if _, err := pool.Exec(ctx, `
UPDATE empire_asset_holdings SET shares = $3, avg_price = $4
WHERE user_id = $1 AND asset_id = $2;`, userID, assetID, newShares, newAvg); err != nil {
		return err
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, proceeds); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "asset_sell", def.ID, proceeds)
	return nil
}

func ActionClaimContract(ctx context.Context, pool *pgxpool.Pool, userID, contractID int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	var progress, target, rm, rx, rr float64
	var status string
	err := pool.QueryRow(ctx, `
SELECT progress, target, reward_money, reward_xp, reward_rp, status
FROM empire_contracts WHERE id = $1 AND user_id = $2;`, contractID, userID).
		Scan(&progress, &target, &rm, &rx, &rr, &status)
	if err != nil {
		return errors.New("contract not found")
	}
	if status != "active" {
		return errors.New("contract is not active")
	}
	if progress < target {
		return errors.New("contract is not completed yet")
	}
	research, err := LoadResearch(ctx, pool, userID)
	if err != nil {
		return err
	}
	boosters, err := LoadBoosters(ctx, pool, userID)
	if err != nil {
		return err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	bonuses := ComputeBonuses(profile, research, boosters, time.Now())
	money := rm * bonuses.ContractRewardMult
	if money > 0 {
		if err := wallet_sql.AdjustBalance(ctx, pool, userID, money); err != nil {
			return err
		}
	}
	profile.RP += rr
	profile.TotalEarned += money
	profile.AllTimeEarned += money
	applyXp(profile, rx)
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
UPDATE empire_contracts SET status = 'claimed' WHERE id = $1;`, contractID); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "contract", fmt.Sprintf("%d", contractID), money)
	return nil
}

func ActionBuyBooster(ctx context.Context, pool *pgxpool.Pool, userID int, boosterType string, durationHours int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	prices, ok := BoosterPrices[boosterType]
	if !ok {
		return errors.New("unknown booster")
	}
	price, ok := prices[durationHours]
	if !ok {
		return errors.New("unknown duration")
	}
	balance, err := wallet_sql.GetBalanceByUserID(ctx, pool, userID)
	if err != nil {
		return err
	}
	if balance < price {
		return errors.New("insufficient funds")
	}
	if err := wallet_sql.AdjustBalance(ctx, pool, userID, -price); err != nil {
		return err
	}
	// Продлеваем от текущего срока, если бустер ещё активен.
	base := time.Now()
	var curExp time.Time
	err = pool.QueryRow(ctx, `
SELECT expires_at FROM empire_boosters
WHERE user_id = $1 AND booster_type = $2;`, userID, boosterType).Scan(&curExp)
	if err == nil && curExp.After(base) {
		base = curExp
	}
	newExp := base.Add(time.Duration(durationHours) * time.Hour)
	if _, err := pool.Exec(ctx, `
INSERT INTO empire_boosters (user_id, booster_type, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, booster_type) DO UPDATE SET expires_at = $3;`,
		userID, boosterType, newExp); err != nil {
		return err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	profile.TotalSpent += price
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "booster", boosterType, price)
	return nil
}

func ActionPrestige(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	if err := Tick(ctx, pool, userID); err != nil {
		return err
	}
	profile, err := GetOrCreateProfile(ctx, pool, userID)
	if err != nil {
		return err
	}
	if profile.Level < PrestigeMinLevel {
		return fmt.Errorf("prestige requires empire level %d", PrestigeMinLevel)
	}
	pp := CalcPrestigePP(profile)
	// Полный сброс забега.
	for _, q := range []string{
		`DELETE FROM empire_objects WHERE user_id = $1;`,
		`DELETE FROM empire_research WHERE user_id = $1;`,
		`DELETE FROM empire_asset_holdings WHERE user_id = $1;`,
		`DELETE FROM empire_contracts WHERE user_id = $1;`,
		`DELETE FROM empire_boosters WHERE user_id = $1;`,
	} {
		if _, err := pool.Exec(ctx, q, userID); err != nil {
			return err
		}
	}
	profile.PrestigeCount++
	profile.PrestigePoints += pp
	profile.Level = 1
	profile.XP = 0
	profile.RP = 0
	profile.TotalEarned = 0
	profile.TotalSpent = 0
	profile.LastTick = time.Now()
	if err := saveProfile(ctx, pool, profile); err != nil {
		return err
	}
	logEvent(ctx, pool, userID, "prestige", "", float64(pp))
	return nil
}

// Гарантируем, что math используется (CalcPrestigePP в state.go).
var _ = math.Floor
