package open_city_sql

import (
	"context"
	"encoding/json"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

type Player struct {
	UserID       int
	DOC          float64
	Level        int
	XP           int
	Health       int
	MaxHealth    int
	PosX         float64
	PosY         float64
	Location     string
	BaseProgress map[string]interface{}
}

// GetOrCreatePlayer — при первом входе создаёт начальное состояние.
func GetOrCreatePlayer(ctx context.Context, pool *pgxpool.Pool, userID int) (*Player, error) {
	p := &Player{UserID: userID, BaseProgress: map[string]interface{}{}}
	var rawProgress []byte
	err := pool.QueryRow(ctx, `
SELECT doc_balance, level, xp, health, max_health,
       pos_x, pos_y, location, base_progress
FROM open_city_state WHERE user_id = $1;`, userID).
		Scan(&p.DOC, &p.Level, &p.XP, &p.Health, &p.MaxHealth,
			&p.PosX, &p.PosY, &p.Location, &rawProgress)
	if err == nil {
		if len(rawProgress) > 0 {
			_ = json.Unmarshal(rawProgress, &p.BaseProgress)
		}
		if p.BaseProgress == nil {
			p.BaseProgress = map[string]interface{}{}
		}
		return p, nil
	}
	_, err = pool.Exec(ctx, `
INSERT INTO open_city_state (user_id, level, xp, health, max_health, location, pos_x, pos_y)
VALUES ($1, $2, $3, $4, $5, 'downtown', 800, 800)
ON CONFLICT (user_id) DO NOTHING;`,
		userID, StartLevel, StartXP, StartHealth, StartMaxHealth)
	if err != nil {
		return nil, err
	}
	p.Level = StartLevel
	p.XP = StartXP
	p.Health = StartHealth
	p.MaxHealth = StartMaxHealth
	p.Location = "downtown"
	p.PosX = 800
	p.PosY = 800
	return p, nil
}

// BuildState — единый полный ответ для frontend одним запросом:
// player + balances + position + inventory + progress.
func BuildState(ctx context.Context, pool *pgxpool.Pool, userID int) (map[string]interface{}, error) {
	player, err := GetOrCreatePlayer(ctx, pool, userID)
	if err != nil {
		return nil, err
	}

	inventory := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT item_id, qty FROM open_city_inventory
WHERE user_id = $1 AND qty > 0
ORDER BY item_id;`, userID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var itemID string
			var qty int
			if err := rows.Scan(&itemID, &qty); err != nil {
				rows.Close()
				return nil, err
			}
			entry := map[string]interface{}{"item_id": itemID, "qty": qty}
			if def := ItemByID[itemID]; def != nil {
				entry["name"] = def.Name
				entry["kind"] = def.Kind
			}
			inventory = append(inventory, entry)
		}
		rows.Close()
	}

	quests := []map[string]interface{}{}
	{
		rows, err := pool.Query(ctx, `
SELECT quest_id, status, progress FROM open_city_quests
WHERE user_id = $1
ORDER BY quest_id;`, userID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var questID, status string
			var raw []byte
			if err := rows.Scan(&questID, &status, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			q := map[string]interface{}{"quest_id": questID, "status": status, "progress": map[string]interface{}{}}
			if len(raw) > 0 {
				var pr map[string]interface{}
				if json.Unmarshal(raw, &pr) == nil {
					q["progress"] = pr
				}
			}
			quests = append(quests, q)
		}
		rows.Close()
	}

	return map[string]interface{}{
		"player": map[string]interface{}{
			"level":      player.Level,
			"xp":         player.XP,
			"xp_next":    XPNeeded(player.Level),
			"health":     player.Health,
			"max_health": player.MaxHealth,
		},
		"balances": map[string]interface{}{
			"doc": round2(player.DOC),
		},
		"position": map[string]interface{}{
			"x":        round2(player.PosX),
			"y":        round2(player.PosY),
			"location": player.Location,
		},
		"inventory": inventory,
		"progress": map[string]interface{}{
			"base":   player.BaseProgress,
			"quests": quests,
		},
	}, nil
}
