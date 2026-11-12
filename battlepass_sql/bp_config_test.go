package battlepass_sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"xwallet-server/profile_sql"
)

func TestBPCardCosmeticPlacement(t *testing.T) {
	want := map[string]string{"free:5": "card-sage", "free:12": "card-coral", "free:20": "card-glacier",
		"pass:8": "card-dusk", "pass:18": "card-aurora", "pass:32": "card-ember", "pass:45": "card-prism"}
	got := map[string]string{}
	for i, row := range BPPassRewards {
		for track, rewards := range map[string][]BPReward{"free": row.Free, "pass": row.Pass} {
			for _, reward := range rewards {
				if reward.Kind != "cosmetic" {
					continue
				}
				cosmetic, ok := profile_sql.GetCosmetic(reward.Cosmetic)
				if !ok || cosmetic.Kind != "card" || cosmetic.AdminOnly || reward.Value != 0 {
					t.Fatalf("invalid card design reward: %+v", reward)
				}
				if reward.Display != cosmetic.Name+" card design" {
					t.Fatalf("wrong presentation: %+v", reward)
				}
				got[fmt.Sprintf("%s:%d", track, i+1)] = reward.Cosmetic
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("placement = %v, want %v", got, want)
	}
	var customs []string
	for _, reward := range BPPassRewards[49].Pass {
		if reward.Kind == "custom" {
			customs = append(customs, reward.Voucher)
		}
	}
	if len(customs) != 1 || customs[0] != "custom_free" {
		t.Fatalf("level 50 reward = %v", customs)
	}
}

func TestBPCaseChancesSumTo100(t *testing.T) {
	for _, rarity := range BPCaseOrder {
		total := 0
		for i, entry := range BPCases[rarity].Entries {
			if entry.Index != i || entry.Chance <= 0 {
				t.Fatalf("%s entry %+v", rarity, entry)
			}
			total += entry.Chance
			if entry.Kind == "cosmetic" {
				if c, ok := profile_sql.GetCosmetic(entry.Cosmetic); !ok || c.Kind != "card" || c.AdminOnly || entry.Value != 0 {
					t.Fatalf("%s cosmetic drop %+v", rarity, entry)
				}
			}
			if entry.Kind == "custom" && entry.Voucher == "" {
				t.Fatalf("%s empty voucher", rarity)
			}
		}
		if total != 100 {
			t.Fatalf("%s total %d", rarity, total)
		}
	}
}

func TestBPCasesDropVouchersAndCards(t *testing.T) {
	counts := map[string]int{}
	for _, rarity := range BPCaseOrder {
		for _, entry := range BPCases[rarity].Entries {
			if entry.Kind == "status" {
				t.Fatal("status drop remains")
			}
			counts[entry.Kind]++
		}
	}
	if counts["custom"] == 0 || counts["cosmetic"] == 0 {
		t.Fatalf("case contents %v", counts)
	}
	if got := BPFormatReward(bpCustom("custom_20")); !strings.Contains(got, "20%") {
		t.Fatalf("display %q", got)
	}
}
