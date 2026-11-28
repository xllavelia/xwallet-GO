package empire_sql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Все действия выполняют тик дохода и собственное изменение в одной
// транзакции. Профиль блокируется до чтения последнего тика.

func ActionBuyObject(ctx context.Context, pool *pgxpool.Pool, userID int, defID string) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		def, ok := EmpireObjectByID[defID]
		if !ok {
			return errors.New("unknown facility")
		}
		if profile.Level < def.UnlockLevel {
			return fmt.Errorf("requires empire level %d", def.UnlockLevel)
		}
		if err := adjustWalletBalanceTx(ctx, tx, userID, -def.BaseCost); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
INSERT INTO empire_objects (user_id, def_id, invested)
VALUES ($1, $2, $3);`, userID, def.ID, def.BaseCost)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "buy empire object"); err != nil {
			return err
		}
		profile.TotalSpent += def.BaseCost
		applyXp(profile, def.BaseCost*XpPerSpent)
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		if err := logEvent(ctx, tx, userID, "purchase", def.ID, def.BaseCost); err != nil {
			return err
		}
		// Прогресс контрактов на покупку объектов.
		if _, err := tx.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + 1
WHERE user_id = $1 AND status = 'active' AND metric = 'objects_bought';`, userID); err != nil {
			return err
		}
		if EmpireRarityOrder[def.Rarity] >= EmpireRarityOrder["rare"] {
			if _, err := tx.Exec(ctx, `
UPDATE empire_contracts SET progress = progress + 1
WHERE user_id = $1 AND status = 'active' AND metric = 'facility_rare';`, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Загружает объект с блокировкой и проверяет, что он принадлежит юзеру.
func loadOwnedObject(ctx context.Context, tx pgx.Tx, userID, objectID int) (*ObjectInstance, *EmpireObjectDef, error) {
	var o ObjectInstance
	err := tx.QueryRow(ctx, `
SELECT id, def_id, COALESCE(custom_name, ''), level, workers_hired,
       worker_xp, satisfaction, invested, lifetime_earned, purchased_at
FROM empire_objects WHERE id = $1 AND user_id = $2 FOR UPDATE;`, objectID, userID).
		Scan(&o.ID, &o.DefID, &o.CustomName, &o.Level, &o.WorkersHired,
			&o.WorkerXP, &o.Satisfaction, &o.Invested, &o.LifetimeEarned, &o.PurchasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errors.New("facility not found")
	}
	if err != nil {
		return nil, nil, err
	}
	def, ok := EmpireObjectByID[o.DefID]
	if !ok {
		return nil, nil, errors.New("unknown facility type")
	}
	return &o, &def, nil
}

func ActionUpgradeObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		o, def, err := loadOwnedObject(ctx, tx, userID, objectID)
		if err != nil {
			return err
		}
		if o.Level >= GlobalMaxLevel {
			return errors.New("facility is at max level")
		}
		cost := def.UpgradeCost(o.Level)
		if err := adjustWalletBalanceTx(ctx, tx, userID, -cost); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
UPDATE empire_objects SET level = level + 1, invested = invested + $2
WHERE id = $1 AND user_id = $3 AND level = $4 AND level < $5;`,
			objectID, cost, userID, o.Level, GlobalMaxLevel)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "upgrade empire object"); err != nil {
			return err
		}
		profile.TotalSpent += cost
		applyXp(profile, cost*XpPerSpent)
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "upgrade", def.ID, cost)
	})
}

func ActionSellObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, _ *EmpireProfile) error {
		o, def, err := loadOwnedObject(ctx, tx, userID, objectID)
		if err != nil {
			return err
		}
		proceeds := o.Invested * SellBackRate
		tag, err := tx.Exec(ctx, `
DELETE FROM empire_objects WHERE id = $1 AND user_id = $2;`, objectID, userID)
		if err != nil {
			return err
		}
		// Награда начисляется только после успешного удаления единственной
		// заблокированной записи.
		if err := requireOneAffected(tag, "sell empire object"); err != nil {
			return err
		}
		if err := adjustWalletBalanceTx(ctx, tx, userID, proceeds); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "sale", def.ID, proceeds)
	})
}

func ActionRenameObject(ctx context.Context, pool *pgxpool.Pool, userID, objectID int, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return errors.New("name must be 1-64 characters")
	}
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, _ *EmpireProfile) error {
		tag, err := tx.Exec(ctx, `
UPDATE empire_objects SET custom_name = $2
WHERE id = $1 AND user_id = $3;`, objectID, name, userID)
		if err != nil {
			return err
		}
		return requireOneAffected(tag, "rename empire object")
	})
}

func ActionHireWorker(ctx context.Context, pool *pgxpool.Pool, userID, objectID int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		o, def, err := loadOwnedObject(ctx, tx, userID, objectID)
		if err != nil {
			return err
		}
		if o.WorkersHired >= def.WorkerSlots {
			return errors.New("all worker slots are filled")
		}
		cost := def.BaseCost * WorkerHireCostRate
		if err := adjustWalletBalanceTx(ctx, tx, userID, -cost); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
UPDATE empire_objects SET workers_hired = workers_hired + 1
WHERE id = $1 AND user_id = $2 AND workers_hired < $3;`, objectID, userID, def.WorkerSlots)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "hire empire worker"); err != nil {
			return err
		}
		profile.TotalSpent += cost
		applyXp(profile, cost*XpPerSpent)
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "hire", def.ID, cost)
	})
}

func ActionBuyResearch(ctx context.Context, pool *pgxpool.Pool, userID int, direction string) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		def, ok := EmpireResearchByID[direction]
		if !ok {
			return errors.New("unknown research direction")
		}
		research, err := LoadResearch(ctx, tx, userID)
		if err != nil {
			return err
		}
		cur := research[direction]
		if cur >= def.MaxLevel {
			return errors.New("research is at max level")
		}
		money := def.MoneyCost(cur)
		rp := def.RPCost(cur)
		if profile.RP < rp {
			return errors.New("not enough research points")
		}
		if err := adjustWalletBalanceTx(ctx, tx, userID, -money); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
INSERT INTO empire_research (user_id, direction, level)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, direction) DO UPDATE SET level = empire_research.level + 1
WHERE empire_research.level < $3;`, userID, direction, def.MaxLevel)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "buy empire research"); err != nil {
			return err
		}
		profile.RP -= rp
		profile.TotalSpent += money
		applyXp(profile, money*XpPerResearchSpent)
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "research", direction, money)
	})
}

func ActionBuyAsset(ctx context.Context, pool *pgxpool.Pool, userID int, assetID string, shares int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, _ *EmpireProfile) error {
		def, ok := EmpireAssetByID[assetID]
		if !ok {
			return errors.New("unknown asset")
		}
		if shares < 1 || shares > 1000000 {
			return errors.New("invalid shares amount")
		}
		var price float64
		if err := tx.QueryRow(ctx, `
SELECT price FROM empire_asset_prices WHERE asset_id = $1 FOR UPDATE;`, assetID).Scan(&price); err != nil {
			return err
		}
		cost := price * float64(shares)
		if err := adjustWalletBalanceTx(ctx, tx, userID, -cost); err != nil {
			return err
		}
		// Пересчёт средней цены покупки выполняется атомарно вместе со списанием.
		tag, err := tx.Exec(ctx, `
INSERT INTO empire_asset_holdings (user_id, asset_id, shares, avg_price)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, asset_id) DO UPDATE SET
    avg_price = (empire_asset_holdings.avg_price * empire_asset_holdings.shares + $4 * $3)
                / (empire_asset_holdings.shares + $3),
    shares = empire_asset_holdings.shares + $3;`, userID, assetID, shares, price)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "buy empire asset"); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "asset_buy", def.ID, cost)
	})
}

func ActionSellAsset(ctx context.Context, pool *pgxpool.Pool, userID int, assetID string, shares int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, _ *EmpireProfile) error {
		def, ok := EmpireAssetByID[assetID]
		if !ok {
			return errors.New("unknown asset")
		}
		if shares < 1 {
			return errors.New("invalid shares amount")
		}
		var owned int
		var avg float64
		err := tx.QueryRow(ctx, `
SELECT shares, avg_price FROM empire_asset_holdings
WHERE user_id = $1 AND asset_id = $2 FOR UPDATE;`, userID, assetID).Scan(&owned, &avg)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("not enough shares")
		}
		if err != nil {
			return err
		}
		if owned < shares {
			return errors.New("not enough shares")
		}
		var price float64
		if err := tx.QueryRow(ctx, `
SELECT price FROM empire_asset_prices WHERE asset_id = $1 FOR UPDATE;`, assetID).Scan(&price); err != nil {
			return err
		}
		proceeds := price * float64(shares) * (1 - AssetTradeFeeRate)
		tag, err := tx.Exec(ctx, `
UPDATE empire_asset_holdings
SET shares = shares - $3,
    avg_price = CASE WHEN shares - $3 = 0 THEN 0 ELSE avg_price END
WHERE user_id = $1 AND asset_id = $2 AND shares >= $3;`, userID, assetID, shares)
		if err != nil {
			return err
		}
		// Повторная продажа не сможет пройти условие shares >= requested.
		if err := requireOneAffected(tag, "sell empire asset"); err != nil {
			return err
		}
		if err := adjustWalletBalanceTx(ctx, tx, userID, proceeds); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "asset_sell", def.ID, proceeds)
	})
}

func ActionClaimContract(ctx context.Context, pool *pgxpool.Pool, userID, contractID int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		var progress, target, rm, rx, rr float64
		var status string
		err := tx.QueryRow(ctx, `
SELECT progress, target, reward_money, reward_xp, reward_rp, status
FROM empire_contracts WHERE id = $1 AND user_id = $2 FOR UPDATE;`, contractID, userID).
			Scan(&progress, &target, &rm, &rx, &rr, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("contract not found")
		}
		if err != nil {
			return err
		}
		if status != "active" {
			return errors.New("contract is not active")
		}
		if progress < target {
			return errors.New("contract is not completed yet")
		}
		research, err := LoadResearch(ctx, tx, userID)
		if err != nil {
			return err
		}
		boosters, err := LoadBoosters(ctx, tx, userID)
		if err != nil {
			return err
		}
		bonuses := ComputeBonuses(profile, research, boosters, time.Now())
		money := rm * bonuses.ContractRewardMult
		tag, err := tx.Exec(ctx, `
UPDATE empire_contracts SET status = 'claimed'
WHERE id = $1 AND user_id = $2 AND status = 'active' AND progress >= target;`, contractID, userID, target)
		if err != nil {
			return err
		}
		// Статус меняется ровно один раз и только после этого выдаётся награда.
		if err := requireOneAffected(tag, "claim empire contract"); err != nil {
			return err
		}
		if money > 0 {
			if err := adjustWalletBalanceTx(ctx, tx, userID, money); err != nil {
				return err
			}
		}
		profile.RP += rr
		profile.TotalEarned += money
		profile.AllTimeEarned += money
		applyXp(profile, rx)
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "contract", fmt.Sprintf("%d", contractID), money)
	})
}

func ActionBuyBooster(ctx context.Context, pool *pgxpool.Pool, userID int, boosterType string, durationHours int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		prices, ok := BoosterPrices[boosterType]
		if !ok {
			return errors.New("unknown booster")
		}
		price, ok := prices[durationHours]
		if !ok {
			return errors.New("unknown duration")
		}
		if err := adjustWalletBalanceTx(ctx, tx, userID, -price); err != nil {
			return err
		}
		// Продлеваем от текущего срока, если бустер ещё активен.
		base := time.Now()
		var curExp time.Time
		err := tx.QueryRow(ctx, `
SELECT expires_at FROM empire_boosters
WHERE user_id = $1 AND booster_type = $2 FOR UPDATE;`, userID, boosterType).Scan(&curExp)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && curExp.After(base) {
			base = curExp
		}
		newExp := base.Add(time.Duration(durationHours) * time.Hour)
		tag, err := tx.Exec(ctx, `
INSERT INTO empire_boosters (user_id, booster_type, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, booster_type) DO UPDATE SET expires_at = $3;`,
			userID, boosterType, newExp)
		if err != nil {
			return err
		}
		if err := requireOneAffected(tag, "buy empire booster"); err != nil {
			return err
		}
		profile.TotalSpent += price
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "booster", boosterType, price)
	})
}

func ActionPrestige(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	return withEmpireTransaction(ctx, pool, userID, func(ctx context.Context, tx pgx.Tx, profile *EmpireProfile) error {
		if profile.Level < PrestigeMinLevel {
			return fmt.Errorf("prestige requires empire level %d", PrestigeMinLevel)
		}
		pp := CalcPrestigePP(profile)
		// Полный сброс забега. Все связанные строки уже заблокированы тиком.
		for _, q := range []string{
			`DELETE FROM empire_objects WHERE user_id = $1;`,
			`DELETE FROM empire_research WHERE user_id = $1;`,
			`DELETE FROM empire_asset_holdings WHERE user_id = $1;`,
			`DELETE FROM empire_contracts WHERE user_id = $1;`,
			`DELETE FROM empire_boosters WHERE user_id = $1;`,
		} {
			if _, err := tx.Exec(ctx, q, userID); err != nil {
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
		if err := saveProfile(ctx, tx, profile); err != nil {
			return err
		}
		return logEvent(ctx, tx, userID, "prestige", "", float64(pp))
	})
}
