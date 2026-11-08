package battlepass_sql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"xwallet-server/profile_sql"
)

func TestBPLegacyStatusCosmetics(t *testing.T) {
	for _, test := range []struct {
		status, cosmetic, legacyLabel string
		fallback                      float64
	}{
		{"lucky", "theme-sage", "Lucky", 25},
		{"valter", "theme-dusk", "Valter", 100},
		{"chicken_burger", "emblem-fox", "Chicken Burger", 15},
		{"silver", "theme-silver", "Silver", 20},
		{"gold", "theme-gold", "Gold", 50},
		{"platinum", "theme-platinum", "Platinum", 150},
		{"legend", "emblem-dragon", "Legend", 500},
		{"owner", "theme-supernova", "Owner", 2000},
		{"first", "emblem-crown", "First", 1000},
	} {
		t.Run(test.status, func(t *testing.T) {
			reward := bpStatus(test.status)
			cosmetic, ok := profile_sql.GetCosmetic(test.cosmetic)
			if !ok || cosmetic.AdminOnly {
				t.Fatalf("invalid mapped cosmetic %s", test.cosmetic)
			}
			if reward.Kind != "status" || reward.Status != test.status || reward.Cosmetic != test.cosmetic || reward.Value != test.fallback {
				t.Fatalf("legacy ownership/fallback changed: %+v", reward)
			}
			if reward.Label != cosmetic.Name || reward.CosmeticKind != cosmetic.Kind || reward.Display != cosmetic.Name+" "+cosmetic.Kind {
				t.Fatalf("wrong cosmetic presentation: %+v", reward)
			}
			if bpStatusLabel(test.status) != test.legacyLabel {
				t.Fatal("historical duplicate-note label changed")
			}
			if got := BPFormatReward(BPReward{Kind: "status", Status: test.status, Display: test.legacyLabel + " status"}); got != reward.Display {
				t.Fatalf("historical status display not mapped: %s", got)
			}
		})
	}
}

func TestBPCosmeticCatalogAndPlacement(t *testing.T) {
	want := map[string]string{
		"free:5": "theme-sage", "free:10": "pattern-leaf", "free:12": "theme-coral",
		"free:15": "emblem-comet", "free:20": "theme-glacier",
		"pass:8": "theme-dusk", "pass:10": "emblem-crystal", "pass:14": "pattern-spark",
		"pass:18": "theme-aurora", "pass:32": "theme-ember", "pass:36": "pattern-constellation", "pass:45": "theme-prism",
	}
	got := map[string]string{}
	for i, row := range BPPassRewards {
		for track, rewards := range map[string][]BPReward{"free": row.Free, "pass": row.Pass} {
			for _, reward := range rewards {
				if reward.Kind != "status" && reward.Kind != "cosmetic" {
					continue
				}
				assertBPCosmetic(t, reward.Cosmetic, reward.CosmeticKind, reward.Label, reward.Display)
				if reward.Kind == "cosmetic" {
					if reward.Value != 0 || reward.Status != "" {
						t.Fatalf("direct cosmetic has cash/status semantics: %+v", reward)
					}
					key := fmt.Sprintf("%s:%d", track, i+1)
					if _, exists := got[key]; exists {
						t.Fatalf("extra direct unlock at %s", key)
					}
					got[key] = reward.Cosmetic
				}
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unlock placement = %v, want %v", got, want)
	}
	var crown bool
	for _, reward := range BPLevelRewardsFor(50).Pass {
		crown = crown || (reward.Kind == "status" && reward.Status == "first" && reward.Cosmetic == "emblem-crown")
	}
	if !crown {
		t.Fatal("premium level 50 lost its legacy crown")
	}
}

func assertBPCosmetic(t *testing.T, id, kind, label, display string) {
	t.Helper()
	cosmetic, ok := profile_sql.GetCosmetic(id)
	if !ok || cosmetic.AdminOnly || strings.Contains(id, "xlavelia") || strings.Contains(id, "sigil") || strings.Contains(id, "monogram") {
		t.Fatalf("invalid/admin-only drop %q", id)
	}
	if kind != cosmetic.Kind || label != cosmetic.Name || display != cosmetic.Name+" "+cosmetic.Kind {
		t.Fatalf("wrong cosmetic metadata for %s: %s/%s/%s", id, kind, label, display)
	}
}

// Compare all pre-existing rewards, not just cash totals, so added unlocks
// cannot silently replace a voucher, currency, status, or case reward.
func TestBPLegacyLevelRewardsUnchanged(t *testing.T) {
	want := [][2]string{
		{"case:epic", "case:legendary"},
		{"cash:5", "cash:10"},
		{"ref:5", "ref:10"},
		{"lavx:2", "lavx:4"},
		{"fee:10:2", "fee:20:4"},
		{"case:common", "case:rare"},
		{"cash:10", "cash:20"},
		{"ref:10", "ref:20"},
		{"cash:15", "cash:30"},
		{"case:rare;cash:5", "case:epic;cash:10"},
		{"lavx:3", "lavx:6"},
		{"cash:20", "cash:40"},
		{"fee:10:3", "fee:20:3"},
		{"case:common", "case:rare"},
		{"cash:25", "cash:50"},
		{"ref:15", "ref:30"},
		{"cash:30", "cash:60"},
		{"case:rare", "case:epic"},
		{"lavx:5", "lavx:10"},
		{"case:epic", "case:mythic"},
		{"cash:35", "cash:70"},
		{"fee:20:4", "fee:40:4"},
		{"ref:20", "ref:40"},
		{"cash:40", "cash:80"},
		{"status:lucky;cash:10", "status:valter;cash:20"},
		{"case:rare", "case:epic"},
		{"cash:45", "cash:90"},
		{"lavx:8", "lavx:16"},
		{"cash:50", "cash:100"},
		{"case:epic;cash:25", "case:mythic;cash:50"},
		{"fee:30:5", "fee:60:5"},
		{"ref:30", "ref:60"},
		{"cash:60", "cash:120"},
		{"case:rare", "case:epic"},
		{"cash:75", "cash:150"},
		{"fee:40:5", "fee:80:5"},
		{"cash:80", "cash:160"},
		{"lavx:10", "lavx:20"},
		{"cash:90", "cash:180"},
		{"status:chicken_burger;case:rare", "status:gold;case:mythic"},
		{"cash:100", "cash:200"},
		{"fee:50:7", "fee:100:7"},
		{"ref:30", "ref:60"},
		{"case:mythic", "case:legendary"},
		{"cash:120", "cash:240"},
		{"fee:100:7", "fee:200:10"},
		{"cash:150", "cash:300"},
		{"lavx:15", "lavx:30"},
		{"cash:200", "cash:400"},
		{"case:legendary", "case:chromatic;status:first"},
	}
	if len(BPPassRewards) != len(want) {
		t.Fatalf("level count = %d", len(BPPassRewards))
	}
	for i, row := range BPPassRewards {
		got := [2]string{bpLegacySignature(row.Free), bpLegacySignature(row.Pass)}
		if got != want[i] {
			t.Fatalf("level %d legacy rewards = %v, want %v", i+1, got, want[i])
		}
	}
}

func bpLegacySignature(rewards []BPReward) string {
	parts := []string{}
	for _, reward := range rewards {
		if reward.Kind == "cosmetic" {
			continue
		}
		part := reward.Kind + ":"
		switch reward.Kind {
		case "status":
			part += reward.Status
		case "case":
			part += reward.Case
		default:
			part += bpNum(reward.Value)
			if reward.Kind == "fee" {
				part += fmt.Sprintf(":%d", reward.Days)
			}
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ";")
}

func TestBPCaseChancesAndLegacyPayouts(t *testing.T) {
	want := map[string]string{
		"common":    "cash:5@20;ref:20@20;lavx:3@20;fee:20:4@20;cash:10@10;cosmetic:emblem-fox@5;fee:100:7@3;status:valter@2",
		"rare":      "cash:10@17;ref:40@14;lavx:6@19;fee:40:4@14;cash:25@9;ref:25@7;cosmetic:pattern-waves@5;cosmetic:theme-cocoa@4;cosmetic:theme-olive@4;fee:200:7@3;status:silver@2;cash:50@1;status:chicken_burger@1",
		"epic":      "cash:25@18;ref:80@15;lavx:10@18;fee:100:5@15;cash:50@10;status:lucky@8;fee:300:10@5;status:gold@4;lavx:20@3;cash:100@2;status:valter@2",
		"mythic":    "cash:50@18;ref:120@15;lavx:15@18;fee:250:7@15;cash:100@10;status:gold@6;fee:500:10@5;lavx:30@4;status:valter@3;cash:250@3;status:chicken_burger@2;fee:1000:30@1",
		"legendary": "cash:100@18;ref:160@15;lavx:20@18;fee:500:10@15;cash:250@10;status:platinum@5;fee:1000:30@5;lavx:50@4;status:valter@3;cash:500@3;status:chicken_burger@2;cash:1000@1;status:legend@1",
		"chromatic": "cash:500@20;ref:200@20;lavx:30@20;fee:2000:30@20;lavx:70@4;cash:700@5;fee:5000:30@5;lavx:100@3;cash:2000@1;status:owner@1;cosmetic:emblem-eclipse@1",
	}
	for _, rarity := range BPCaseOrder {
		def := BPCases[rarity]
		parts := []string{}
		total := 0
		for i, entry := range def.Entries {
			if entry.Index != i || entry.Chance <= 0 {
				t.Fatalf("invalid %s case entry %+v", rarity, entry)
			}
			total += entry.Chance
			part := bpLegacySignature([]BPReward{{Kind: entry.Kind, Value: entry.Value, Days: entry.Days, Status: entry.Status}})
			if entry.Kind == "status" || entry.Kind == "cosmetic" {
				assertBPCosmetic(t, entry.Cosmetic, entry.CosmeticKind, entry.Label, entry.Display)
			}
			if entry.Kind == "cosmetic" {
				if entry.Value != 0 || entry.Status != "" {
					t.Fatalf("direct case cosmetic can pay cash: %+v", entry)
				}
				part = "cosmetic:" + entry.Cosmetic
			}
			parts = append(parts, fmt.Sprintf("%s@%d", part, entry.Chance))
		}
		if total != 100 || strings.Join(parts, ";") != want[rarity] {
			t.Fatalf("%s chances/payouts changed: total=%d, entries=%s", rarity, total, strings.Join(parts, ";"))
		}
	}
}

func TestBPCaseRollPersistsCosmetic(t *testing.T) {
	entry := bpCaseCosmetic("emblem-eclipse", 1)
	roll := BPCaseRoll{Entry: entry, Index: 10, Cosmetic: entry.Cosmetic, Granted: entry.Display}
	raw, err := json.Marshal(roll)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BPCaseRoll
	if err := json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(decoded, roll) {
		t.Fatalf("cosmetic roll did not round-trip: %+v, %v", decoded, err)
	}
	if !strings.Contains(string(raw), `"cosmetic":"emblem-eclipse"`) {
		t.Fatal("cosmetic ID missing from persisted roll")
	}
	var legacy BPCaseRoll
	if err := json.Unmarshal([]byte(`{"entry":{"kind":"status","status":"owner","value":2000},"index":9,"note":"duplicate Owner status → $2000","granted":"duplicate Owner status → $2000"}`), &legacy); err != nil || legacy.Entry.Status != "owner" || legacy.Note != "duplicate Owner status → $2000" {
		t.Fatalf("legacy roll no longer decodes: %+v, %v", legacy, err)
	}
}
