package open_city_sql

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================
// ВСЕ ОПЕРАЦИИ С DOC — ТОЛЬКО СЕРВЕРНЫЕ.
// Frontend никогда не присылает суммы: он отправляет id награды
// или серверные действия, а размер и проверки живут здесь.
// Любое изменение DOC идёт через атомарный UPDATE с RETURNING.
// То же касается XP и характеристик: клиент присылает только
// ключи источников, всё считает сервер.
// ============================================================

// awardDOC — внутреннее начисление (будущие миссии, квесты, NPC).
// Экспортировано для следующих этапов; из handlers не вызывается напрямую.
func awardDOC(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) error {
	if amount <= 0 {
		return nil
	}
	var balance float64
	err := pool.QueryRow(ctx, `
UPDATE open_city_state
SET doc_balance = doc_balance + $2,
    updated_at = now()
WHERE user_id = $1 AND doc_balance + $2 >= 0
RETURNING doc_balance;`, userID, amount).Scan(&balance)
	if err != nil {
		return errors.New("doc update failed")
	}
	return nil
}

// spendDOC — списание с проверкой достатка.
func spendDOC(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) error {
	if amount <= 0 {
		return errors.New("invalid amount")
	}
	var balance float64
	err := pool.QueryRow(ctx, `
UPDATE open_city_state
SET doc_balance = doc_balance - $2,
    updated_at = now()
WHERE user_id = $1 AND doc_balance >= $2
RETURNING doc_balance;`, userID, amount).Scan(&balance)
	if err != nil {
		return errors.New("insufficient doc")
	}
	return nil
}

// ActionMove — сохранение позиции. Координаты клэмпятся в границы
// локации из каталога; неизвестная локация отклоняется.
func ActionMove(ctx context.Context, pool *pgxpool.Pool, userID int, location string, x, y float64) error {
	loc := LocationByID[location]
	if loc == nil {
		return errors.New("unknown location")
	}
	if x < loc.MinX {
		x = loc.MinX
	}
	if x > loc.MaxX {
		x = loc.MaxX
	}
	if y < loc.MinY {
		y = loc.MinY
	}
	if y > loc.MaxY {
		y = loc.MaxY
	}
	_, err := pool.Exec(ctx, `
UPDATE open_city_state
SET pos_x = $2, pos_y = $3, location = $4, updated_at = now()
WHERE user_id = $1;`, userID, x, y, location)
	return err
}

// ActionClaimReward — разовая награда из каталога. Размер награды
// знает только сервер; повторная выдача той же награды отклоняется.
func ActionClaimReward(ctx context.Context, pool *pgxpool.Pool, userID int, rewardID string) error {
	reward := RewardByID[rewardID]
	if reward == nil {
		return errors.New("unknown reward")
	}
	player, err := GetOrCreatePlayer(ctx, pool, userID)
	if err != nil {
		return err
	}

	claimed, _ := player.BaseProgress["claimed"].([]interface{})
	for _, c := range claimed {
		if s, ok := c.(string); ok && s == rewardID {
			return errors.New("reward already claimed")
		}
	}

	// атомарно: размер из каталога + защита от ухода в минус
	var balance float64
	err = pool.QueryRow(ctx, `
UPDATE open_city_state
SET doc_balance = doc_balance + $2,
    base_progress = jsonb_set(
        COALESCE(base_progress, '{}'::jsonb),
        '{claimed}',
        COALESCE(base_progress->'claimed', '[]'::jsonb) || to_jsonb($3::text),
        true
    ),
    updated_at = now()
WHERE user_id = $1 AND doc_balance + $2 >= 0
RETURNING doc_balance;`, userID, float64(reward.Doc), rewardID).Scan(&balance)
	if err != nil {
		return errors.New("reward failed")
	}
	return nil
}

// ActionInventory — добавление/удаление предметов. Каталог валидирует
// item_id; удаление — атомарным UPDATE с условием qty >= n.
func ActionInventory(ctx context.Context, pool *pgxpool.Pool, userID int, op, itemID string, qty int) error {
	if qty < 1 || qty > MaxInventoryQty {
		return errors.New("invalid qty")
	}
	if ItemByID[itemID] == nil {
		return errors.New("unknown item")
	}

	switch op {
	case "add":
		_, err := pool.Exec(ctx, `
INSERT INTO open_city_inventory (user_id, item_id, qty)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, item_id) DO UPDATE SET
    qty = LEAST(open_city_inventory.qty + EXCLUDED.qty, $4);`,
			userID, itemID, qty, MaxInventoryQty)
		return err

	case "remove":
		var left int
		err := pool.QueryRow(ctx, `
UPDATE open_city_inventory SET qty = qty - $3
WHERE user_id = $1 AND item_id = $2 AND qty >= $3
RETURNING qty;`, userID, itemID, qty).Scan(&left)
		if err != nil {
			return errors.New("not enough items")
		}
		if left == 0 {
			_, _ = pool.Exec(ctx, `
DELETE FROM open_city_inventory WHERE user_id = $1 AND item_id = $2 AND qty = 0;`,
				userID, itemID)
		}
		return nil

	default:
		return errors.New("unknown inventory op")
	}
}

// ActionSaveProgress — слияние base_progress. Клиент присылает
// частичный объект; сервер пропускает только разрешённые ключи
// (AllowedProgressKeys) и режет тело по лимиту байт.
func ActionSaveProgress(ctx context.Context, pool *pgxpool.Pool, userID int, patch map[string]interface{}) error {
	if len(patch) == 0 {
		return nil
	}
	clean := map[string]interface{}{}
	for k, v := range patch {
		if AllowedProgressKeys[k] {
			clean[k] = v
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return errors.New("invalid progress")
	}
	if len(raw) > MaxProgressJSON {
		return errors.New("progress too large")
	}
	_, err = pool.Exec(ctx, `
UPDATE open_city_state
SET base_progress = COALESCE(base_progress, '{}'::jsonb) || $2::jsonb,
    updated_at = now()
WHERE user_id = $1;`, userID, string(raw))
	return err
}

// ============================================================
// ПРОКАЧКА: XP, характеристики, экипировка.
// Все значения считает сервер; клиент присылает только ключи.
// ============================================================

// ActionGainXP — начисление XP по ключу источника из каталога.
// Левелапы, очки навыков и рост Max HP считает сервер; при
// левелапе игрок лечится до полного.
func ActionGainXP(ctx context.Context, pool *pgxpool.Pool, userID int, source string) error {
	xp, ok := XPRewards[source]
	if !ok {
		return errors.New("unknown xp source")
	}
	var curXP, level, skillPoints, health int
	var rawStats []byte
	err := pool.QueryRow(ctx, `
SELECT xp, level, skill_points, health, stats
FROM open_city_state WHERE user_id = $1;`, userID).
		Scan(&curXP, &level, &skillPoints, &health, &rawStats)
	if err != nil {
		return err
	}
	pts := map[string]int{}
	if len(rawStats) > 0 {
		_ = json.Unmarshal(rawStats, &pts)
	}
	curXP += xp
	leveled := false
	for curXP >= XPNeeded(level) {
		curXP -= XPNeeded(level)
		level++
		skillPoints += SkillPointsPerLevel
		leveled = true
	}
	maxHealth := StartMaxHealth + (level-1)*LevelMaxHPGrowth + pts["max_hp"]*MaxHPPerPoint
	if leveled {
		health = maxHealth // полное лечение при левелапе
	} else if health > maxHealth {
		health = maxHealth
	}
	_, err = pool.Exec(ctx, `
UPDATE open_city_state
SET xp = $2, level = $3, skill_points = $4, max_health = $5, health = $6, updated_at = now()
WHERE user_id = $1;`, userID, curXP, level, skillPoints, maxHealth, health)
	return err
}

// ActionAllocateSkill — вложить одно очко навыка в характеристику
// из StatDefs. Max HP пересчитывается сразу, текущее HP клэмпится.
func ActionAllocateSkill(ctx context.Context, pool *pgxpool.Pool, userID int, stat string) error {
	if !StatByID[stat] {
		return errors.New("unknown stat")
	}
	var skillPoints, level, health int
	var rawStats []byte
	err := pool.QueryRow(ctx, `
SELECT skill_points, level, health, stats FROM open_city_state WHERE user_id = $1;`, userID).
		Scan(&skillPoints, &level, &health, &rawStats)
	if err != nil {
		return err
	}
	if skillPoints < 1 {
		return errors.New("no skill points")
	}
	pts := map[string]int{}
	if len(rawStats) > 0 {
		_ = json.Unmarshal(rawStats, &pts)
	}
	pts[stat]++
	raw, err := json.Marshal(pts)
	if err != nil {
		return errors.New("invalid stats")
	}
	maxHealth := StartMaxHealth + (level-1)*LevelMaxHPGrowth + pts["max_hp"]*MaxHPPerPoint
	if health > maxHealth {
		health = maxHealth
	}
	_, err = pool.Exec(ctx, `
UPDATE open_city_state
SET skill_points = $2, stats = $3::jsonb, max_health = $4, health = $5, updated_at = now()
WHERE user_id = $1;`, userID, skillPoints-1, string(raw), maxHealth, health)
	return err
}

// ActionEquipWeapon — экипировать оружие из инвентаря ("" — убрать).
// Проверка: предмет из каталога kind=weapon и есть в инвентаре
// в количестве >= 1. Хранится отдельно от предметов — переключение
// оружия в будущем меняет только эту колонку.
func ActionEquipWeapon(ctx context.Context, pool *pgxpool.Pool, userID int, itemID string) error {
	if itemID != "" {
		def := ItemByID[itemID]
		if def == nil || def.Kind != "weapon" {
			return errors.New("not a weapon")
		}
		var qty int
		err := pool.QueryRow(ctx, `
SELECT qty FROM open_city_inventory WHERE user_id = $1 AND item_id = $2;`, userID, itemID).Scan(&qty)
		if err != nil || qty < 1 {
			return errors.New("weapon not in inventory")
		}
	}
	_, err := pool.Exec(ctx, `
UPDATE open_city_state SET equipped_weapon = $2, updated_at = now() WHERE user_id = $1;`, userID, itemID)
	return err
}
