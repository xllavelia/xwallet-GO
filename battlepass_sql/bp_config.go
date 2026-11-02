package battlepass_sql

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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
	Kind    string  `json:"kind"`
	Value   float64 `json:"value,omitempty"`
	Days    int     `json:"days,omitempty"`
	Case    string  `json:"case,omitempty"`
	Status  string  `json:"status,omitempty"`
	Label   string  `json:"label,omitempty"`
	Display string  `json:"display,omitempty"`
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

func bpStatus(status string) BPReward {
	label := bpStatusLabel(status)
	return BPReward{Kind: "status", Status: status, Label: label, Value: BPStatusFallback[status], Display: label + " status"}
}

var BPStatusFallback = map[string]float64{
	"lucky":          25,
	"valter":         100,
	"chicken_burger": 15,
	"silver":         20,
	"gold":           50,
	"platinum":       150,
	"legend":         500,
	"owner":          2000,
	"first":          1000,
}

func bpStatusLabel(status string) string {
	switch status {
	case "lucky":
		return "Lucky"
	case "valter":
		return "Valter"
	case "chicken_burger":
		return "Chicken Burger"
	case "silver":
		return "Silver"
	case "gold":
		return "Gold"
	case "platinum":
		return "Platinum"
	case "legend":
		return "Legend"
	case "owner":
		return "Owner"
	case "first":
		return "First"
	}
	return strings.Title(strings.ReplaceAll(status, "_", " "))
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
	case "status":
		return bpStatusLabel(r.Status) + " status"
	}
	return r.Kind
}

var BPPassRewards = []BPLevelRewards{
	{Free: []BPReward{bpCase("epic")}, Pass: []BPReward{bpCase("legendary")}},
	{Free: []BPReward{bpCash(5)}, Pass: []BPReward{bpCash(10)}},
	{Free: []BPReward{bpRef(5)}, Pass: []BPReward{bpRef(10)}},
	{Free: []BPReward{bpLavx(2)}, Pass: []BPReward{bpLavx(4)}},
	{Free: []BPReward{bpFee(10, 2)}, Pass: []BPReward{bpFee(20, 4)}},
	{Free: []BPReward{bpCase("common")}, Pass: []BPReward{bpCase("rare")}},
	{Free: []BPReward{bpCash(10)}, Pass: []BPReward{bpCash(20)}},
	{Free: []BPReward{bpRef(10)}, Pass: []BPReward{bpRef(20)}},
	{Free: []BPReward{bpCash(15)}, Pass: []BPReward{bpCash(30)}},
	{Free: []BPReward{bpCase("rare"), bpCash(5)}, Pass: []BPReward{bpCase("epic"), bpCash(10)}},
	{Free: []BPReward{bpLavx(3)}, Pass: []BPReward{bpLavx(6)}},
	{Free: []BPReward{bpCash(20)}, Pass: []BPReward{bpCash(40)}},
	{Free: []BPReward{bpFee(10, 3)}, Pass: []BPReward{bpFee(20, 3)}},
	{Free: []BPReward{bpCase("common")}, Pass: []BPReward{bpCase("rare")}},
	{Free: []BPReward{bpCash(25)}, Pass: []BPReward{bpCash(50)}},
	{Free: []BPReward{bpRef(15)}, Pass: []BPReward{bpRef(30)}},
	{Free: []BPReward{bpCash(30)}, Pass: []BPReward{bpCash(60)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic")}},
	{Free: []BPReward{bpLavx(5)}, Pass: []BPReward{bpLavx(10)}},
	{Free: []BPReward{bpCase("epic")}, Pass: []BPReward{bpCase("mythic")}},
	{Free: []BPReward{bpCash(35)}, Pass: []BPReward{bpCash(70)}},
	{Free: []BPReward{bpFee(20, 4)}, Pass: []BPReward{bpFee(40, 4)}},
	{Free: []BPReward{bpRef(20)}, Pass: []BPReward{bpRef(40)}},
	{Free: []BPReward{bpCash(40)}, Pass: []BPReward{bpCash(80)}},
	{Free: []BPReward{bpStatus("lucky"), bpCash(10)}, Pass: []BPReward{bpStatus("valter"), bpCash(20)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic")}},
	{Free: []BPReward{bpCash(45)}, Pass: []BPReward{bpCash(90)}},
	{Free: []BPReward{bpLavx(8)}, Pass: []BPReward{bpLavx(16)}},
	{Free: []BPReward{bpCash(50)}, Pass: []BPReward{bpCash(100)}},
	{Free: []BPReward{bpCase("epic"), bpCash(25)}, Pass: []BPReward{bpCase("mythic"), bpCash(50)}},
	{Free: []BPReward{bpFee(30, 5)}, Pass: []BPReward{bpFee(60, 5)}},
	{Free: []BPReward{bpRef(30)}, Pass: []BPReward{bpRef(60)}},
	{Free: []BPReward{bpCash(60)}, Pass: []BPReward{bpCash(120)}},
	{Free: []BPReward{bpCase("rare")}, Pass: []BPReward{bpCase("epic")}},
	{Free: []BPReward{bpCash(75)}, Pass: []BPReward{bpCash(150)}},
	{Free: []BPReward{bpFee(40, 5)}, Pass: []BPReward{bpFee(80, 5)}},
	{Free: []BPReward{bpCash(80)}, Pass: []BPReward{bpCash(160)}},
	{Free: []BPReward{bpLavx(10)}, Pass: []BPReward{bpLavx(20)}},
	{Free: []BPReward{bpCash(90)}, Pass: []BPReward{bpCash(180)}},
	{Free: []BPReward{bpStatus("chicken_burger"), bpCase("rare")}, Pass: []BPReward{bpStatus("gold"), bpCase("mythic")}},
	{Free: []BPReward{bpCash(100)}, Pass: []BPReward{bpCash(200)}},
	{Free: []BPReward{bpFee(50, 7)}, Pass: []BPReward{bpFee(100, 7)}},
	{Free: []BPReward{bpRef(30)}, Pass: []BPReward{bpRef(60)}},
	{Free: []BPReward{bpCase("mythic")}, Pass: []BPReward{bpCase("legendary")}},
	{Free: []BPReward{bpCash(120)}, Pass: []BPReward{bpCash(240)}},
	{Free: []BPReward{bpFee(100, 7)}, Pass: []BPReward{bpFee(200, 10)}},
	{Free: []BPReward{bpCash(150)}, Pass: []BPReward{bpCash(300)}},
	{Free: []BPReward{bpLavx(15)}, Pass: []BPReward{bpLavx(30)}},
	{Free: []BPReward{bpCash(200)}, Pass: []BPReward{bpCash(400)}},
	{Free: []BPReward{bpCase("legendary")}, Pass: []BPReward{bpCase("chromatic"), bpStatus("first")}},
}

func BPLevelRewardsFor(level int) BPLevelRewards {
	if level < 1 || level > len(BPPassRewards) {
		return BPLevelRewards{Free: []BPReward{}, Pass: []BPReward{}}
	}
	return BPPassRewards[level-1]
}

type BPCaseEntry struct {
	Index   int     `json:"index"`
	Kind    string  `json:"kind"`
	Value   float64 `json:"value,omitempty"`
	Days    int     `json:"days,omitempty"`
	Status  string  `json:"status,omitempty"`
	Label   string  `json:"label,omitempty"`
	Display string  `json:"display"`
	Chance  int     `json:"chance"`
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

func bpc(kind string, value float64, days int, status string, chance int) BPCaseEntry {
	e := BPCaseEntry{Kind: kind, Value: value, Days: days, Status: status, Chance: chance}
	switch kind {
	case "cash":
		e.Display = "$" + bpNum(value)
	case "lavx":
		e.Display = bpNum(value) + " LAVX"
	case "ref":
		e.Display = bpNum(value) + " REF XP"
	case "fee":
		e.Display = fmt.Sprintf("$%s fee · %dd", bpNum(value), days)
	case "status":
		e.Label = bpStatusLabel(status)
		e.Display = e.Label + " status"
	}
	return e
}

func bpBuildCases() map[string]BPCaseDef {
	defs := map[string]BPCaseDef{
		"common": {
			Rarity: "common", Title: "Common case", Subtitle: "A small drop from the bottom shelf.",
			Entries: []BPCaseEntry{
				bpc("cash", 5, 0, "", 20),
				bpc("ref", 20, 0, "", 20),
				bpc("lavx", 3, 0, "", 20),
				bpc("fee", 20, 4, "", 20),
				bpc("cash", 10, 0, "", 10),
				bpc("status", 0, 0, "lucky", 5),
				bpc("fee", 100, 7, "", 3),
				bpc("status", 0, 0, "valter", 2),
			},
		},
		"rare": {
			Rarity: "rare", Title: "Rare case", Subtitle: "Odds get a little more interesting.",
			Entries: []BPCaseEntry{
				bpc("cash", 10, 0, "", 20),
				bpc("ref", 40, 0, "", 15),
				bpc("lavx", 6, 0, "", 20),
				bpc("fee", 40, 4, "", 15),
				bpc("cash", 25, 0, "", 10),
				bpc("ref", 25, 0, "", 8),
				bpc("status", 0, 0, "lucky", 5),
				bpc("fee", 200, 7, "", 3),
				bpc("status", 0, 0, "silver", 2),
				bpc("cash", 50, 0, "", 1),
				bpc("status", 0, 0, "chicken_burger", 1),
			},
		},
		"epic": {
			Rarity: "epic", Title: "Epic case", Subtitle: "Built for people who keep playing.",
			Entries: []BPCaseEntry{
				bpc("cash", 25, 0, "", 18),
				bpc("ref", 80, 0, "", 15),
				bpc("lavx", 10, 0, "", 18),
				bpc("fee", 100, 5, "", 15),
				bpc("cash", 50, 0, "", 10),
				bpc("status", 0, 0, "lucky", 8),
				bpc("fee", 300, 10, "", 5),
				bpc("status", 0, 0, "gold", 4),
				bpc("lavx", 20, 0, "", 3),
				bpc("cash", 100, 0, "", 2),
				bpc("status", 0, 0, "valter", 2),
			},
		},
		"mythic": {
			Rarity: "mythic", Title: "Mythic case", Subtitle: "The middle of the good stuff.",
			Entries: []BPCaseEntry{
				bpc("cash", 50, 0, "", 18),
				bpc("ref", 120, 0, "", 15),
				bpc("lavx", 15, 0, "", 18),
				bpc("fee", 250, 7, "", 15),
				bpc("cash", 100, 0, "", 10),
				bpc("status", 0, 0, "gold", 6),
				bpc("fee", 500, 10, "", 5),
				bpc("lavx", 30, 0, "", 4),
				bpc("status", 0, 0, "valter", 3),
				bpc("cash", 250, 0, "", 3),
				bpc("status", 0, 0, "chicken_burger", 2),
				bpc("fee", 1000, 30, "", 1),
			},
		},
		"legendary": {
			Rarity: "legendary", Title: "Legendary case", Subtitle: "One good click can change the season.",
			Entries: []BPCaseEntry{
				bpc("cash", 100, 0, "", 18),
				bpc("ref", 160, 0, "", 15),
				bpc("lavx", 20, 0, "", 18),
				bpc("fee", 500, 10, "", 15),
				bpc("cash", 250, 0, "", 10),
				bpc("status", 0, 0, "platinum", 5),
				bpc("fee", 1000, 30, "", 5),
				bpc("lavx", 50, 0, "", 4),
				bpc("status", 0, 0, "valter", 3),
				bpc("cash", 500, 0, "", 3),
				bpc("status", 0, 0, "chicken_burger", 2),
				bpc("cash", 1000, 0, "", 1),
				bpc("status", 0, 0, "legend", 1),
			},
		},
		"chromatic": {
			Rarity: "chromatic", Title: "Chromatic case", Subtitle: "The top of the season. No filler.",
			Entries: []BPCaseEntry{
				bpc("cash", 500, 0, "", 20),
				bpc("ref", 200, 0, "", 20),
				bpc("lavx", 30, 0, "", 20),
				bpc("fee", 2000, 30, "", 20),
				bpc("lavx", 70, 0, "", 5),
				bpc("cash", 700, 0, "", 5),
				bpc("fee", 5000, 30, "", 5),
				bpc("lavx", 100, 0, "", 3),
				bpc("cash", 2000, 0, "", 1),
				bpc("status", 0, 0, "owner", 1),
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
