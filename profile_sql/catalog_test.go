package profile_sql

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogContract(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 18 {
		t.Fatalf("catalog size = %d", len(catalog))
	}
	seen := map[string]bool{}
	for _, c := range catalog {
		if seen[c.ID] || !strings.HasPrefix(c.ID, "card-") || c.Kind != "card" {
			t.Fatalf("invalid/nonunique cosmetic: %+v", c)
		}
		seen[c.ID] = true
		if c.Owned || c.AdminOnly != (c.Source == "admin") {
			t.Fatalf("catalog must not bake in ownership: %+v", c)
		}
		if (c.Source == "shop") != (c.Price > 0) {
			t.Fatalf("unexpected price: %+v", c)
		}
		if c.Source == "battlepass" && (c.Level < 1 || c.Level > 50 || (c.Track != "free" && c.Track != "pass")) {
			t.Fatalf("bad battle pass cosmetic: %+v", c)
		}
		if len(c.Colors) != 2 {
			t.Fatalf("card design needs left color and ink: %+v", c)
		}
		for _, color := range c.Colors {
			if !strings.HasPrefix(color, "#") || len(color) != 7 {
				t.Fatalf("bad design color %q", color)
			}
		}
	}
	wantLevels := map[string]struct {
		level int
		track string
	}{
		"card-sage": {5, "free"}, "card-coral": {12, "free"}, "card-glacier": {20, "free"},
		"card-dusk": {8, "pass"}, "card-aurora": {18, "pass"}, "card-ember": {32, "pass"}, "card-prism": {45, "pass"},
	}
	for id, want := range wantLevels {
		c, ok := GetCosmetic(id)
		if !ok || c.Level != want.level || c.Track != want.track {
			t.Fatalf("contract changed for %s: %+v", id, c)
		}
	}
	for id, price := range map[string]int{"card-silk": 50, "card-nebula": 100, "card-iridescent": 200} {
		c, ok := GetCosmetic(id)
		if !ok || c.Price != price {
			t.Fatalf("bad shop price %s", id)
		}
	}
	catalog[0].Name = "mutated"
	if Catalog()[0].Name == "mutated" {
		t.Fatal("catalog leaked backing slice")
	}
}

func TestOwnershipContract(t *testing.T) {
	owned := map[string]bool{"card-sage": true, "card-gold": true}
	for _, c := range Catalog() {
		if !HasCosmetic(c.ID, nil, true) {
			t.Fatalf("administrator missing cosmetic: %s", c.ID)
		}
		if c.AdminOnly {
			if HasCosmetic(c.ID, owned, false) || !HasCosmetic(c.ID, nil, true) {
				t.Fatal("admin access violation")
			}
		} else if owned[c.ID] {
			if !HasCosmetic(c.ID, owned, false) {
				t.Fatalf("owned not available: %s", c.ID)
			}
		} else if HasCosmetic(c.ID, owned, false) {
			t.Fatalf("unowned available: %s", c.ID)
		}
	}
	if HasCosmetic("invalid", owned, true) {
		t.Fatal("admin can equip nonexistent cosmetic")
	}
	for _, c := range CatalogFor(owned, false) {
		if c.AdminOnly && c.Owned {
			t.Fatal("catalog leaked admin ownership")
		}
		if c.Owned != owned[c.ID] {
			t.Fatalf("catalog ownership wrong for %s", c.ID)
		}
	}
}

func TestCardDesignColors(t *testing.T) {
	colors, ok := CardDesignColors("card-gold")
	if !ok || colors[0] == "" || colors[1] == "" {
		t.Fatal("card-gold colors missing")
	}
	if _, ok := CardDesignColors("theme-gold"); ok {
		t.Fatal("legacy id accepted")
	}
	if key, ok := NormalizeCardDesignKey(" Card-Gold "); !ok || key != "card-gold" {
		t.Fatal("normalization failed")
	}
	if _, ok := NormalizeCardDesignKey("theme-gold"); ok {
		t.Fatal("non-card kind accepted")
	}
}

func TestPublicProfileRedaction(t *testing.T) {
	balance := 123.0
	p := PublicProfile{PlayerID: "123456", Username: "Player", ShowBalance: false, Balance: nil, FriendState: "none"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err = json.Unmarshal(data, &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"balance", "lavxBalance", "passwordHash", "password_hash", "userId", "profit24h", "deviceId", "banReason", "themeId", "emblemId", "patternId"} {
		if _, ok := obj[key]; ok {
			t.Fatalf("private property: %s", key)
		}
	}
	if len(obj) != 9 {
		t.Fatalf("unexpected public schema: %s", data)
	}
	p.ShowBalance, p.Balance = true, &balance
	data, _ = json.Marshal(p)
	if !strings.Contains(string(data), `"balance":123`) {
		t.Fatal("visible balance omitted")
	}
}
