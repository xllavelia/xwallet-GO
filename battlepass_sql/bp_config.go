package battlepass_sql

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"xwallet-server/profile_sql"
)

const (
	BPXpPerLevel          = 1000
	BPMaxLevel            = 50
	BPPassPriceLavx       = 250.0
	BPSeasonLabel         = "Season 1"
	BPSeasonDuration      = 60 * 24 * time.Hour
	BPVoucherSlots        = 5
	BPDuplicateStatusCash = 50.0
)

type BPReward struct {
	Kind         string  `json:"kind"`
	Value        float64 `json:"value,omitempty"`
	Days         int     `json:"days,omitempty"`
	Case         string  `json:"case,omitempty"`
	Voucher      string  `json:"voucher,omitempty"`
	Cosmetic     string  `json:"cosmetic,omitempty"`
	CosmeticKind string  `json:"cosmeticKind,omitempty"`
	Label        string  `json:"label,omitempty"`
	Display      string  `json:"display,omitempty"`
}

type BPLevelRewards struct {
	Free []BPReward `json:"free"`
	Pass []BPReward `json:"pass"`
}

func bpNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func bpCash(v float64) BPReward {
	return BPReward{Kind: "cash", Value: v, Display: "$" + bpNum(v)}
}

func bpLavx(v float64) BPReward {
	return BPReward{Kind: "lavx", Value: v, Display: bpNum(v) + " LAVX"}
}

func bpRef(v float64) BPReward {
	return BPReward{Kind: "ref", Value: v, Display: bpNum(v) + " REF XP"}
}

func bpFee(v float64, days int) BPReward {
	return BPReward{Kind: "fee", Value: v, Days: days, Display: fmt.Sprintf("$%s fee · %dd", bpNum(v), days)}
}

func bpCase(rarity string) BPReward {
	return BPReward{Kind: "case", Case: rarity, Display: bpCaseTitle(rarity) + " case"}
}

func bpCosmetic(id string) BPReward {
	cosmetic, ok := profile_sql.GetCosmetic(id)
	if !ok || cosmetic.AdminOnly {
		panic(fmt.Sprintf("invalid battle pass cosmetic %q", id))
	}
	return BPReward{
		Kind: "cosmetic", Cosmetic: cosmetic.ID, CosmeticKind: cosmetic.Kind,
		Label: cosmetic.Name, Display: cosmetic.Name + " card design",
	}
}

// bpCustom — ваучер на кастомизацию карточки: бесплатно или скидка 20/50%.
func bpCustom(voucher string) BPReward {
	switch voucher {
	case "custom_free":
		return BPReward{Kind: "custom", Voucher: voucher, Label: "Free customization", Display: "Free card customization"}
	case "custom_20":
		return BPReward{Kind: "custom", Voucher: voucher, Value: 20, Label: "Customization -20%", Display: "Card customization -20%"}
	case "custom_50":
		return BPReward{Kind: "custom", Voucher: voucher, Value: 50, Label: "Customization -50%", Display: "Card customization -50%"}
	}
	panic(fmt.Sprintf("invalid custom voucher %q", voucher))
}

func bpCaseTitle(rarity string) string {
	switch rarity {
	case "common":
		return "Common"
	case "rare":
		return "Rare"
	case "epic":
		return "Epic"
	case "mythic":
		return "Mythic"
	case "legendary":
		return "Legendary"
	case "chromatic":
		return "Chromatic"
	}
	return strings.Title(rarity)
}

func BPFormatReward(r BPReward) string {
	if r.Kind == "cosmetic" {
		if cosmetic, ok := profile_sql.GetCosmetic(r.Cosmetic); ok {
			return cosmetic.Name + " card design"
		}
	}
	if r.Display != "" {
		return r.Display
	}
	switch r.Kind {
	case "cash":
		return "$" + bpNum(r.Value)
	case "lavx":
		return bpNum(r.Value) + " LAVX"
	case "ref":
		return bpNum(r.Value) + " REF XP"
	case "fee":
		return fmt.Sprintf("$%s fee · %dd", bpNum(r.Value), r.Days)
	case "case":
		return bpCaseTitle(r.Case) + " case"
	}
	return r.Kind
}

var BPPassRewards = []BPLevelRewards{
	{Free: []BPReward{bpCase("epic")}, Pass: []BPReward{bpCase("legendary")}},
	{Free: []BPReward{bpCash(5)}, Pass: []BPReward{bpCash(10)}},
	{Free: []BPReward{bpRef(5)}, Pass: []BPReward{bpRef(10)}},
	{Free: []BPReward{bpLavx(2)}, Pass: []BPReward{bpLavx(4)}},
	{Free: []BPReward{bpFee(10, 2), bpCosmetic("card-sage")}, Pass: []BPReward{bpFee(20, 4)}},
	{Free: []BPReward{bpCase("common")}, Pass: []BPReward{bpCase("rare")}},
	{Free: []BPReward{bpCash(10)}, Pass: []BPReward{bpCash(20)}},
	{Free: []BPReward{bpRef(10)}, Pass: []BPReward{bpRef(20), bpCosmetic("card-dusk")}},
	{Free: []BPReward{bpCash(15)}, Pass: []BPReward{bpCash(30)}},
	{Free: []BPReward{bpCase("rare"), bpCash(5)}, Pass: []BPReward{bpCase("epic"), bpCash(10)}},
	{Free: []BPReward{bpLavx(3)}, Pass: []BPReward{bpLavx(6)}},
	{Free: []BPReward{bpCash(20), bpCosmetic("card-coral")}, Pass: []BPReward{bpCash(40)}},
	{Free: []BPReward{bpFee(10, 3)}, Pass: []BPReward{bpFee(20, 3)}},
	{Free: []BPReward{bpCase("common")}, Pass: []BPReward{bpCase("rare"), bpCustom("custom_20")}},
	{Free: []BPReward{bpCash(25)}, Pass: []BPReward{bpCash(50)}},
	{Free: []BPReward{bpRef(15)}, Pass: []BPReward{bpRef(30)}},
	{Free: []BPReward{bpCash(30)}, Pass: []BPReward{bpCash(60)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic"), bpCosmetic("card-aurora")}},
	{Free: []BPReward{bpLavx(5)}, Pass: []BPReward{bpLavx(10)}},
	{Free: []BPReward{bpCase("epic"), bpCosmetic("card-glacier")}, Pass: []BPReward{bpCase("mythic")}},
	{Free: []BPReward{bpCash(35)}, Pass: []BPReward{bpCash(70)}},
	{Free: []BPReward{bpFee(20, 4)}, Pass: []BPReward{bpFee(40, 4)}},
	{Free: []BPReward{bpRef(20)}, Pass: []BPReward{bpRef(40)}},
	{Free: []BPReward{bpCash(40)}, Pass: []BPReward{bpCash(80)}},
	{Free: []BPReward{bpCustom("custom_20"), bpCash(10)}, Pass: []BPReward{bpCustom("custom_50"), bpCash(20)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic")}},
	{Free: []BPReward{bpCash(45)}, Pass: []BPReward{bpCash(90)}},
	{Free: []BPReward{bpLavx(8)}, Pass: []BPReward{bpLavx(16)}},
	{Free: []BPReward{bpCash(50)}, Pass: []BPReward{bpCash(100)}},
	{Free: []BPReward{bpCase("epic"), bpCash(25)}, Pass: []BPReward{bpCase("mythic"), bpCash(50)}},
	{Free: []BPReward{bpFee(30, 5)}, Pass: []BPReward{bpFee(60, 5)}},
	{Free: []BPReward{bpRef(30)}, Pass: []BPReward{bpRef(60), bpCosmetic("card-ember")}},
	{Free: []BPReward{bpCash(60)}, Pass: []BPReward{bpCash(120)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic")}},
	{Free: []BPReward{bpCash(75)}, Pass: []BPReward{bpCash(150)}},
	{Free: []BPReward{bpFee(40, 5)}, Pass: []BPReward{bpFee(80, 5)}},
	{Free: []BPReward{bpCash(80)}, Pass: []BPReward{bpCash(160)}},
	{Free: []BPReward{bpLavx(10)}, Pass: []BPReward{bpLavx(20)}},
	{Free: []BPReward{bpCash(90)}, Pass: []BPReward{bpCash(180)}},
	{Free: []BPReward{bpCustom("custom_free"), bpCase("rare")}, Pass: []BPReward{bpCustom("custom_50"), bpCase("mythic")}},
	{Free: []BPReward{bpCash(100)}, Pass: []BPReward{bpCash(200)}},
	{Free: []BPReward{bpFee(50, 7)}, Pass: []BPReward{bpFee(100, 7)}},
	{Free: []BPReward{bpRef(30)}, Pass: []BPReward{bpRef(60)}},
	{Free: []BPReward{bpCase("mythic")}, Pass: []BPReward{bpCase("legendary")}},
	{Free: []BPReward{bpCash(120)}, Pass: []BPReward{bpCash(240), bpCosmetic("card-prism")}},
	{Free: []BPReward{bpFee(100, 7)}, Pass: []BPReward{bpFee(200, 10)}},
	{Free: []BPReward{bpCash(150)}, Pass: []BPReward{bpCash(300)}},
	{Free: []BPReward{bpLavx(15)}, Pass: []BPReward{bpLavx(30)}},
	{Free: []BPReward{bpCash(200)}, Pass: []BPReward{bpCash(400)}},
	{Free: []BPReward{bpCase("legendary")}, Pass: []BPReward{bpCase("chromatic"), bpCustom("custom_free")}},
}

func BPLevelRewardsFor(level int) BPLevelRewards {
	if level < 1 || level > len(BPPassRewards) {
		return BPLevelRewards{Free: []BPReward{}, Pass: []BPReward{}}
	}
	return BPPassRewards[level-1]
}

type BPCaseEntry struct {
	Index        int     `json:"index"`
	Kind         string  `json:"kind"`
	Value        float64 `json:"value,omitempty"`
	Days         int     `json:"days,omitempty"`
	Voucher      string  `json:"voucher,omitempty"`
	Cosmetic     string  `json:"cosmetic,omitempty"`
	CosmeticKind string  `json:"cosmeticKind,omitempty"`
	Label        string  `json:"label,omitempty"`
	Display      string  `json:"display"`
	Chance       int     `json:"chance"`
}

type BPCaseDef struct {
	Rarity   string        `json:"rarity"`
	Title    string        `json:"title"`
	Subtitle string        `json:"subtitle"`
	Entries  []BPCaseEntry `json:"entries"`
}

type BPCaseCatalog struct {
	Order []string             `json:"order"`
	Cases map[string]BPCaseDef `json:"cases"`
}

var BPCaseOrder = []string{"common", "rare", "epic", "mythic", "legendary", "chromatic"}

var BPCases = bpBuildCases()

func bpc(kind string, value float64, days int, chance int) BPCaseEntry {
	e := BPCaseEntry{Kind: kind, Value: value, Days: days, Chance: chance}
	switch kind {
	case "cash":
		e.Display = "$" + bpNum(value)
	case "lavx":
		e.Display = bpNum(value) + " LAVX"
	case "ref":
		e.Display = bpNum(value) + " REF XP"
	case "fee":
		e.Display = fmt.Sprintf("$%s fee · %dd", bpNum(value), days)
	}
	return e
}

func bpCustomEntry(voucher string, chance int) BPCaseEntry {
	reward := bpCustom(voucher)
	return BPCaseEntry{
		Kind: reward.Kind, Voucher: reward.Voucher, Value: reward.Value,
		Label: reward.Label, Display: reward.Display, Chance: chance,
	}
}

func bpCaseCosmetic(id string, chance int) BPCaseEntry {
	reward := bpCosmetic(id)
	return BPCaseEntry{
		Kind: reward.Kind, Cosmetic: reward.Cosmetic, CosmeticKind: reward.CosmeticKind,
		Label: reward.Label, Display: reward.Display, Chance: chance,
	}
}

func bpBuildCases() map[string]BPCaseDef {
	defs := map[string]BPCaseDef{
		"common": {
			Rarity: "common", Title: "Common case", Subtitle: "A small drop from the bottom shelf.",
			Entries: []BPCaseEntry{
				bpc("cash", 5, 0, 20),
				bpc("ref", 20, 0, 20),
				bpc("lavx", 3, 0, 20),
				bpc("fee", 20, 4, 20),
				bpc("cash", 10, 0, 10),
				bpCustomEntry("custom_20", 5),
				bpc("fee", 100, 7, 3),
				bpCustomEntry("custom_50", 2),
			},
		},
		"rare": {
			Rarity: "rare", Title: "Rare case", Subtitle: "Odds get a little more interesting.",
			Entries: []BPCaseEntry{
				bpc("cash", 10, 0, 17),
				bpc("ref", 40, 0, 14),
				bpc("lavx", 6, 0, 19),
				bpc("fee", 40, 4, 14),
				bpc("cash", 25, 0, 9),
				bpc("ref", 25, 0, 7),
				bpCaseCosmetic("card-silver", 5),
				bpCaseCosmetic("card-cocoa", 4),
				bpCaseCosmetic("card-olive", 4),
				bpc("fee", 200, 7, 3),
				bpCustomEntry("custom_50", 2),
				bpc("cash", 50, 0, 1),
				bpCustomEntry("custom_20", 1),
			},
		},
		"epic": {
			Rarity: "epic", Title: "Epic case", Subtitle: "Built for people who keep playing.",
			Entries: []BPCaseEntry{
				bpc("cash", 25, 0, 18),
				bpc("ref", 80, 0, 15),
				bpc("lavx", 10, 0, 18),
				bpc("fee", 100, 5, 15),
				bpc("cash", 50, 0, 10),
				bpCaseCosmetic("card-gold", 8),
				bpc("fee", 300, 10, 5),
				bpCustomEntry("custom_50", 4),
				bpc("lavx", 20, 0, 3),
				bpc("cash", 100, 0, 2),
				bpCustomEntry("custom_20", 2),
			},
		},
		"mythic": {
			Rarity: "mythic", Title: "Mythic case", Subtitle: "The middle of the good stuff.",
			Entries: []BPCaseEntry{
				bpc("cash", 50, 0, 18),
				bpc("ref", 120, 0, 15),
				bpc("lavx", 15, 0, 18),
				bpc("fee", 250, 7, 15),
				bpc("cash", 100, 0, 10),
				bpCaseCosmetic("card-platinum", 6),
				bpc("fee", 500, 10, 5),
				bpc("lavx", 30, 0, 4),
				bpCustomEntry("custom_50", 3),
				bpc("cash", 250, 0, 3),
				bpCustomEntry("custom_20", 2),
				bpc("fee", 1000, 30, 1),
			},
		},
		"legendary": {
			Rarity: "legendary", Title: "Legendary case", Subtitle: "One good click can change the season.",
			Entries: []BPCaseEntry{
				bpc("cash", 100, 0, 18),
				bpc("ref", 160, 0, 15),
				bpc("lavx", 20, 0, 18),
				bpc("fee", 500, 10, 15),
				bpc("cash", 250, 0, 10),
				bpCaseCosmetic("card-supernova", 5),
				bpc("fee", 1000, 30, 5),
				bpc("lavx", 50, 0, 4),
				bpCustomEntry("custom_50", 3),
				bpc("cash", 500, 0, 3),
				bpCustomEntry("custom_20", 2),
				bpc("cash", 1000, 0, 1),
				bpCustomEntry("custom_free", 1),
			},
		},
		"chromatic": {
			Rarity: "chromatic", Title: "Chromatic case", Subtitle: "The top of the season. No filler.",
			Entries: []BPCaseEntry{
				bpc("cash", 500, 0, 20),
				bpc("ref", 200, 0, 20),
				bpc("lavx", 30, 0, 20),
				bpc("fee", 2000, 30, 20),
				bpc("lavx", 70, 0, 4),
				bpc("cash", 700, 0, 5),
				bpc("fee", 5000, 30, 5),
				bpc("lavx", 100, 0, 3),
				bpc("cash", 2000, 0, 1),
				bpCustomEntry("custom_free", 1),
				bpCaseCosmetic("card-supernova", 1),
			},
		},
	}
	for _, rarity := range BPCaseOrder {
		def := defs[rarity]
		for i := range def.Entries {
			def.Entries[i].Index = i
		}
		defs[rarity] = def
	}
	return defs
}
