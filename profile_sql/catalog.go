package profile_sql

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Cosmetic описывает дизайны карточек: цвет левой части и цвет текста.
type Cosmetic struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Rarity    string   `json:"rarity"`
	Source    string   `json:"source"`
	Price     int      `json:"price"`
	Level     int      `json:"level,omitempty"`
	Track     string   `json:"track,omitempty"`
	AdminOnly bool     `json:"adminOnly,omitempty"`
	Colors    []string `json:"colors,omitempty"`
	Owned     bool     `json:"owned"`
}

// CardDesignColors возвращает [цвет левой части, цвет текста] для дизайна карточки.
func CardDesignColors(id string) ([2]string, bool) {
	var out [2]string
	c, ok := GetCosmetic(id)
	if !ok || c.Kind != "card" || len(c.Colors) < 2 {
		return out, false
	}
	out[0], out[1] = c.Colors[0], c.Colors[1]
	return out, true
}

var cosmetics = []Cosmetic{
	// Battle Pass: яркие и тусклые дизайны карточек вместо цветов профиля.
	{ID: "card-sage", Kind: "card", Name: "Sage", Rarity: "common", Source: "battlepass", Level: 5, Track: "free", Colors: []string{"#788b43", "#ffffff"}},
	{ID: "card-coral", Kind: "card", Name: "Coral", Rarity: "rare", Source: "battlepass", Level: 12, Track: "free", Colors: []string{"#e45d55", "#ffffff"}},
	{ID: "card-glacier", Kind: "card", Name: "Glacier", Rarity: "epic", Source: "battlepass", Level: 20, Track: "free", Colors: []string{"#75cbdc", "#101010"}},
	{ID: "card-dusk", Kind: "card", Name: "Dusk", Rarity: "rare", Source: "battlepass", Level: 8, Track: "pass", Colors: []string{"#895b83", "#ffffff"}},
	{ID: "card-aurora", Kind: "card", Name: "Aurora", Rarity: "epic", Source: "battlepass", Level: 18, Track: "pass", Colors: []string{"#24a879", "#ffffff"}},
	{ID: "card-ember", Kind: "card", Name: "Ember", Rarity: "legendary", Source: "battlepass", Level: 32, Track: "pass", Colors: []string{"#d8793f", "#ffffff"}},
	{ID: "card-prism", Kind: "card", Name: "Prism", Rarity: "chromatic", Source: "battlepass", Level: 45, Track: "pass", Colors: []string{"#db65b5", "#ffffff"}},
	// Кейсы: редкие тона.
	{ID: "card-silver", Kind: "card", Name: "Silver", Rarity: "rare", Source: "case", Colors: []string{"#c0c7d1", "#101010"}},
	{ID: "card-gold", Kind: "card", Name: "Gold", Rarity: "epic", Source: "case", Colors: []string{"#e9bd62", "#21180d"}},
	{ID: "card-platinum", Kind: "card", Name: "Platinum", Rarity: "legendary", Source: "case", Colors: []string{"#e4e5e8", "#20232b"}},
	{ID: "card-supernova", Kind: "card", Name: "Supernova", Rarity: "chromatic", Source: "case", Colors: []string{"#f05b8b", "#ffffff"}},
	{ID: "card-cocoa", Kind: "card", Name: "Cocoa", Rarity: "rare", Source: "case", Colors: []string{"#5c4230", "#efe0c9"}},
	{ID: "card-olive", Kind: "card", Name: "Olive", Rarity: "rare", Source: "case", Colors: []string{"#66703c", "#e6e3c6"}},
	// Магазин: эксклюзивы за LAVX.
	{ID: "card-silk", Kind: "card", Name: "Silk", Rarity: "rare", Source: "shop", Price: 50, Colors: []string{"#e5b49e", "#ffffff"}},
	{ID: "card-nebula", Kind: "card", Name: "Nebula", Rarity: "epic", Source: "shop", Price: 100, Colors: []string{"#9271e9", "#ffffff"}},
	{ID: "card-iridescent", Kind: "card", Name: "Iridescent", Rarity: "chromatic", Source: "shop", Price: 200, Colors: []string{"#9acbba", "#101010"}},
	// Командные эксклюзивы, видимые только администраторам.
	{ID: "card-xlavelia", Kind: "card", Name: "Xlavelia", Rarity: "exclusive", Source: "admin", AdminOnly: true, Colors: []string{"#013e37", "#ffefb3"}},
	{ID: "card-wave", Kind: "card", Name: "Wave", Rarity: "exclusive", Source: "admin", AdminOnly: true, Colors: []string{"#16324b", "#f3eddd"}},
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

func HasCosmetic(id string, owned map[string]bool, isAdmin bool) bool {
	c, ok := GetCosmetic(id)
	if !ok {
		return false
	}
	if c.AdminOnly {
		return isAdmin // Явная выдача не сохраняет доступ после отзыва роли администратора.
	}
	return isAdmin || owned[id]
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

// NormalizeCardDesignKey приводит ключ к каноническому виду и проверяет, что это дизайн карточки.
func NormalizeCardDesignKey(id string) (string, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	c, ok := GetCosmetic(id)
	if !ok || c.Kind != "card" {
		return "", false
	}
	return id, true
}
