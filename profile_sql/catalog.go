package profile_sql

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Cosmetic struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Rarity    string `json:"rarity"`
	Source    string `json:"source"`
	Price     int    `json:"price"`
	Level     int    `json:"level,omitempty"`
	Track     string `json:"track,omitempty"`
	AdminOnly bool   `json:"adminOnly,omitempty"`
	Owned     bool   `json:"owned"`
}

const (
	DefaultTheme   = "theme-tide"
	DefaultEmblem  = "emblem-none"
	DefaultPattern = "pattern-orbit"
)

var cosmetics = []Cosmetic{
	{ID: "theme-graphite", Kind: "theme", Name: "Graphite", Rarity: "common", Source: "starter"},
	{ID: "theme-tide", Kind: "theme", Name: "Tide", Rarity: "common", Source: "starter"},
	{ID: "theme-sage", Kind: "theme", Name: "Sage", Rarity: "common", Source: "battlepass", Level: 5, Track: "free"},
	{ID: "theme-coral", Kind: "theme", Name: "Coral", Rarity: "rare", Source: "battlepass", Level: 12, Track: "free"},
	{ID: "theme-glacier", Kind: "theme", Name: "Glacier", Rarity: "epic", Source: "battlepass", Level: 20, Track: "free"},
	{ID: "theme-dusk", Kind: "theme", Name: "Dusk", Rarity: "rare", Source: "battlepass", Level: 8, Track: "pass"},
	{ID: "theme-aurora", Kind: "theme", Name: "Aurora", Rarity: "epic", Source: "battlepass", Level: 18, Track: "pass"},
	{ID: "theme-ember", Kind: "theme", Name: "Ember", Rarity: "legendary", Source: "battlepass", Level: 32, Track: "pass"},
	{ID: "theme-prism", Kind: "theme", Name: "Prism", Rarity: "chromatic", Source: "battlepass", Level: 45, Track: "pass"},
	{ID: "theme-silver", Kind: "theme", Name: "Silver", Rarity: "rare", Source: "case"},
	{ID: "theme-gold", Kind: "theme", Name: "Gold", Rarity: "epic", Source: "case"},
	{ID: "theme-platinum", Kind: "theme", Name: "Platinum", Rarity: "legendary", Source: "case"},
	{ID: "theme-supernova", Kind: "theme", Name: "Supernova", Rarity: "chromatic", Source: "case"},
	{ID: "theme-cocoa", Kind: "theme", Name: "Cocoa", Rarity: "rare", Source: "case"},
	{ID: "theme-olive", Kind: "theme", Name: "Olive", Rarity: "rare", Source: "case"},
	{ID: "theme-silk", Kind: "theme", Name: "Silk", Rarity: "rare", Source: "shop", Price: 50},
	{ID: "theme-nebula", Kind: "theme", Name: "Nebula", Rarity: "epic", Source: "shop", Price: 100},
	{ID: "theme-iridescent", Kind: "theme", Name: "Iridescent", Rarity: "chromatic", Source: "shop", Price: 200},
	{ID: "theme-xlavelia", Kind: "theme", Name: "Xlavelia", Rarity: "exclusive", Source: "admin", AdminOnly: true},
	{ID: "theme-wave", Kind: "theme", Name: "Wave", Rarity: "exclusive", Source: "admin", AdminOnly: true},
	{ID: "emblem-none", Kind: "emblem", Name: "None", Rarity: "common", Source: "starter"},
	{ID: "emblem-spark", Kind: "emblem", Name: "Spark", Rarity: "common", Source: "starter"},
	{ID: "emblem-comet", Kind: "emblem", Name: "Comet", Rarity: "rare", Source: "battlepass", Level: 15, Track: "free"},
	{ID: "emblem-crystal", Kind: "emblem", Name: "Crystal", Rarity: "epic", Source: "battlepass", Level: 10, Track: "pass"},
	{ID: "emblem-fox", Kind: "emblem", Name: "Fox", Rarity: "common", Source: "case"},
	{ID: "emblem-crown", Kind: "emblem", Name: "Crown", Rarity: "legendary", Source: "battlepass", Level: 50, Track: "pass"},
	{ID: "emblem-dragon", Kind: "emblem", Name: "Dragon", Rarity: "legendary", Source: "case"},
	{ID: "emblem-eclipse", Kind: "emblem", Name: "Eclipse", Rarity: "chromatic", Source: "case"},
	{ID: "emblem-sigil", Kind: "emblem", Name: "Sigil", Rarity: "exclusive", Source: "admin", AdminOnly: true},
	{ID: "pattern-none", Kind: "pattern", Name: "None", Rarity: "common", Source: "starter"},
	{ID: "pattern-orbit", Kind: "pattern", Name: "Orbit", Rarity: "common", Source: "starter"},
	{ID: "pattern-leaf", Kind: "pattern", Name: "Leaf", Rarity: "common", Source: "battlepass", Level: 10, Track: "free"},
	{ID: "pattern-waves", Kind: "pattern", Name: "Waves", Rarity: "rare", Source: "case"},
	{ID: "pattern-spark", Kind: "pattern", Name: "Spark", Rarity: "epic", Source: "battlepass", Level: 14, Track: "pass"},
	{ID: "pattern-constellation", Kind: "pattern", Name: "Constellation", Rarity: "legendary", Source: "battlepass", Level: 36, Track: "pass"},
	{ID: "pattern-monogram", Kind: "pattern", Name: "Monogram", Rarity: "exclusive", Source: "admin", AdminOnly: true},
}

var legacyCosmetics = map[string]string{
	"lucky": "theme-sage", "valter": "theme-dusk", "chicken_burger": "emblem-fox",
	"silver": "theme-silver", "gold": "theme-gold", "platinum": "theme-platinum",
	"legend": "emblem-dragon", "legendary": "emblem-dragon", "owner": "theme-supernova",
	"first": "emblem-crown", "bronze": "theme-graphite", "diamond": "theme-prism",
}

func Catalog() []Cosmetic { return append([]Cosmetic(nil), cosmetics...) }

func GetCosmetic(id string) (Cosmetic, bool) {
	for _, c := range cosmetics {
		if c.ID == id {
			return c, true
		}
	}
	return Cosmetic{}, false
}

func LegacyCosmetic(status string) string {
	return legacyCosmetics[strings.ToLower(strings.TrimSpace(status))]
}

func HasCosmetic(id string, owned map[string]bool, isAdmin bool) bool {
	c, ok := GetCosmetic(id)
	if !ok {
		return false
	}
	if c.AdminOnly {
		return isAdmin // Явная выдача не сохраняет доступ после отзыва роли администратора.
	}
	return isAdmin || c.Source == "starter" || owned[id]
}

func CatalogFor(owned map[string]bool, isAdmin bool) []Cosmetic {
	result := Catalog()
	for i := range result {
		result[i].Owned = HasCosmetic(result[i].ID, owned, isAdmin)
	}
	return result
}

const grantCosmeticSQL = `INSERT INTO user_cosmetics (user_id, cosmetic_id, source)
VALUES ($1, $2, $3) ON CONFLICT (user_id, cosmetic_id) DO NOTHING`

// Админские эксклюзивы зависят только от роли, поэтому их нельзя выдать явно.
func GrantCosmeticTx(ctx context.Context, tx pgx.Tx, userID int, id, source string) (bool, error) {
	c, ok := GetCosmetic(id)
	if !ok {
		return false, ErrInvalid
	}
	if c.AdminOnly {
		return false, ErrForbidden
	}
	if userID <= 0 || !validText(source, 1, 64) {
		return false, ErrInvalid
	}
	var banned bool
	err := tx.QueryRow(ctx, `SELECT `+bannedSQL+` FROM users u WHERE u.id=$1`, userID).Scan(&banned)
	if err == pgx.ErrNoRows {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if banned {
		return false, ErrForbidden
	}
	tag, err := tx.Exec(ctx, grantCosmeticSQL, userID, id, source)
	return tag.RowsAffected() == 1, err
}
