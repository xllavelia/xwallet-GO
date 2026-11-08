package profile_sql

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogContract(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 36 {
		t.Fatalf("catalog size = %d", len(catalog))
	}
	seen := map[string]bool{}
	for _, c := range catalog {
		if seen[c.ID] || !strings.HasPrefix(c.ID, c.Kind+"-") {
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
	}
	wantLevels := map[string]struct {
		level int
		track string
	}{
		"theme-sage": {5, "free"}, "theme-coral": {12, "free"}, "theme-glacier": {20, "free"},
		"theme-dusk": {8, "pass"}, "theme-aurora": {18, "pass"}, "theme-ember": {32, "pass"}, "theme-prism": {45, "pass"},
		"emblem-comet": {15, "free"}, "emblem-crystal": {10, "pass"}, "emblem-crown": {50, "pass"},
		"pattern-leaf": {10, "free"}, "pattern-spark": {14, "pass"}, "pattern-constellation": {36, "pass"},
	}
	for id, want := range wantLevels {
		c, ok := GetCosmetic(id)
		if !ok || c.Level != want.level || c.Track != want.track {
			t.Fatalf("contract changed for %s: %+v", id, c)
		}
	}
	for id, price := range map[string]int{"theme-silk": 50, "theme-nebula": 100, "theme-iridescent": 200} {
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

func TestOwnershipAndLegacyContract(t *testing.T) {
	want := map[string]string{"lucky": "theme-sage", "valter": "theme-dusk", "chicken_burger": "emblem-fox",
		"silver": "theme-silver", "gold": "theme-gold", "platinum": "theme-platinum", "legend": "emblem-dragon",
		"owner": "theme-supernova", "first": "emblem-crown"}
	owned := map[string]bool{}
	for status, id := range want {
		if LegacyCosmetic(status) != id || LegacyCosmetic(" "+strings.ToUpper(status)+" ") != id {
			t.Fatalf("legacy %s", status)
		}
		owned[id] = true
	}
	if LegacyCosmetic("admin") != "" || LegacyCosmetic("unknown") != "" {
		t.Fatal("unknown status grants access")
	}
	for _, c := range Catalog() {
		if !HasCosmetic(c.ID, nil, true) {
			t.Fatalf("administrator missing cosmetic: %s", c.ID)
		}
		if c.AdminOnly {
			owned[c.ID] = true
			if HasCosmetic(c.ID, owned, false) || !HasCosmetic(c.ID, nil, true) {
				t.Fatal("admin access violation")
			}
			if displayCosmetic(c.ID, c.Kind, "fallback", owned, false) != "fallback" {
				t.Fatal("revoked admin displayed exclusive")
			}
		} else if c.Source == "starter" || owned[c.ID] {
			if !HasCosmetic(c.ID, owned, false) {
				t.Fatalf("owned/starter not available: %s", c.ID)
			}
		} else if HasCosmetic(c.ID, owned, false) {
			t.Fatalf("unowned available: %s", c.ID)
		}
	}
	if HasCosmetic("invalid", owned, true) {
		t.Fatal("admin can equip nonexistent cosmetic")
	}
	if displayCosmetic("emblem-fox", "theme", DefaultTheme, owned, false) != DefaultTheme {
		t.Fatal("kind confusion")
	}
	for _, c := range CatalogFor(owned, false) {
		if c.AdminOnly && c.Owned {
			t.Fatal("catalog leaked admin ownership")
		}
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
	for _, key := range []string{"balance", "lavxBalance", "passwordHash", "password_hash", "userId", "profit24h", "deviceId", "banReason"} {
		if _, ok := obj[key]; ok {
			t.Fatalf("private property: %s", key)
		}
	}
	if len(obj) != 11 {
		t.Fatalf("unexpected public schema: %s", data)
	}
	p.ShowBalance, p.Balance = true, &balance
	data, _ = json.Marshal(p)
	if !strings.Contains(string(data), `"balance":123`) {
		t.Fatal("visible balance omitted")
	}
}
