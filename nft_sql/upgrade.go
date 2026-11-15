package nft_sql

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	UpgradeStep = 2
	UpgradeEdge = 0.85
)

var ErrUpgradeGap = errors.New("target must be two rarities above")

type UpgradeTarget struct {
	TemplateID string          `json:"templateId"`
	Name       string          `json:"name"`
	Rarity     string          `json:"rarity"`
	Art        json.RawMessage `json:"art"`
	Collection CollectionRef   `json:"collection"`
	BasePrice  float64         `json:"basePrice"`
	IncomeHour float64         `json:"incomeHour"`
	Supply     int             `json:"supply"`
	Minted     int             `json:"minted"`
	Chance     float64         `json:"chance"`
}

type UpgradeView struct {
	Item    ItemView        `json:"item"`
	Targets []UpgradeTarget `json:"targets"`
	Step    int             `json:"step"`
	Edge    float64         `json:"edge"`
}

type UpgradeResult struct {
	Won    bool          `json:"won"`
	Chance float64       `json:"chance"`
	Roll   float64       `json:"roll"`
	Source ItemView      `json:"source"`
	Target UpgradeTarget `json:"target"`
	Item   *ItemView     `json:"item,omitempty"`
}

func rarityIndex(key string) int {
	for i, r := range Rarities {
		if r.Key == key {
			return i
		}
	}
	return -1
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// Шанс = стоимость источника к стоимости цели с небольшим преимуществом системы.
func upgradeChance(srcValue, targetValue float64) float64 {
	if targetValue <= 0 {
		return 0
	}
	chance := srcValue / targetValue * UpgradeEdge
	if chance < 0.01 {
		chance = 0.01
	}
	if chance > 0.95 {
		chance = 0.95
	}
	return round4(chance)
}

func upgradeTargets(ctx context.Context, q querier, rarity string, srcValue float64) ([]UpgradeTarget, error) {
	rows, err := q.Query(ctx, `
SELECT t.id, t.name, t.rarity, t.art, c.id, c.name, c.theme,
       t.base_price::float8, t.income_hour::float8, t.supply, t.minted
FROM nft_templates t JOIN nft_collections c ON c.id = t.collection_id
WHERE t.rarity = $1 AND t.minted < t.supply
ORDER BY t.base_price, t.name`, rarity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := []UpgradeTarget{}
	for rows.Next() {
		var t UpgradeTarget
		if err = rows.Scan(&t.TemplateID, &t.Name, &t.Rarity, &t.Art, &t.Collection.ID, &t.Collection.Name,
			&t.Collection.Theme, &t.BasePrice, &t.IncomeHour, &t.Supply, &t.Minted); err != nil {
			return nil, err
		}
		t.Chance = upgradeChance(srcValue, t.BasePrice)
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// UpgradeOptions отдаёт выбранный NFT и все цели ровно на две редкости выше.
func UpgradeOptions(ctx context.Context, pool *pgxpool.Pool, userID int, itemID int64) (*UpgradeView, error) {
	var owner int
	err := pool.QueryRow(ctx, `SELECT owner_id FROM nft_items WHERE id = $1`, itemID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, ErrNotOwner
	}
	item, err := ItemByID(ctx, pool, itemID)
	if err != nil {
		return nil, err
	}
	out := &UpgradeView{Item: *item, Targets: []UpgradeTarget{}, Step: UpgradeStep, Edge: UpgradeEdge}
	idx := rarityIndex(item.Rarity)
	if idx < 0 || idx+UpgradeStep >= len(Rarities) {
		return out, nil
	}
	out.Targets, err = upgradeTargets(ctx, pool, Rarities[idx+UpgradeStep].Key, item.Value)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Upgrade сжигает источник и при удаче минтит выбранную цель кратно выше.
func Upgrade(ctx context.Context, pool *pgxpool.Pool, userID int, itemID int64, templateID string) (*UpgradeResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var owner int
	err = tx.QueryRow(ctx, `SELECT owner_id FROM nft_items WHERE id = $1 FOR UPDATE`, itemID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, ErrNotOwner
	}
	var listed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nft_listings WHERE item_id = $1)`, itemID).Scan(&listed); err != nil {
		return nil, err
	}
	if listed {
		return nil, ErrListed
	}
	src, err := ItemByID(ctx, tx, itemID)
	if err != nil {
		return nil, err
	}
	idx := rarityIndex(src.Rarity)
	if idx < 0 || idx+UpgradeStep >= len(Rarities) {
		return nil, ErrUpgradeGap
	}
	wantRarity := Rarities[idx+UpgradeStep].Key

	var tgt UpgradeTarget
	err = tx.QueryRow(ctx, `
SELECT t.id, t.name, t.rarity, t.art, c.id, c.name, c.theme,
       t.base_price::float8, t.income_hour::float8, t.supply, t.minted
FROM nft_templates t JOIN nft_collections c ON c.id = t.collection_id
WHERE t.id = $1 AND t.rarity = $2 AND t.minted < t.supply
FOR UPDATE OF t`, templateID, wantRarity).
		Scan(&tgt.TemplateID, &tgt.Name, &tgt.Rarity, &tgt.Art, &tgt.Collection.ID, &tgt.Collection.Name,
			&tgt.Collection.Theme, &tgt.BasePrice, &tgt.IncomeHour, &tgt.Supply, &tgt.Minted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUpgradeGap
	}
	if err != nil {
		return nil, err
	}
	tgt.Chance = upgradeChance(src.Value, tgt.BasePrice)
	roll := rand.Float64()
	won := roll < tgt.Chance

	if err = settleTx(ctx, tx, userID, itemID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_items WHERE id = $1`, itemID); err != nil {
		return nil, err
	}
	var granted *ItemView
	var newID *int64
	if won {
		id, _, err := mintTx(ctx, tx, userID, tgt.TemplateID, "Upgrade")
		if err != nil {
			return nil, err
		}
		view, err := ItemByID(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		granted, newID = view, &id
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_upgrades(user_id, source_template_id, source_serial, source_value,
                         target_template_id, target_rarity, target_value, chance, roll, won, item_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		userID, src.TemplateID, src.Serial, src.Value, tgt.TemplateID, tgt.Rarity, tgt.BasePrice,
		tgt.Chance, roll, won, newID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &UpgradeResult{Won: won, Chance: tgt.Chance, Roll: roll, Source: *src, Target: tgt, Item: granted}, nil
}
