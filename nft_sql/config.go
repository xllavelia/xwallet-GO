package nft_sql

import (
	"errors"
	"math"
	"time"
)

const (
	InstantRate     = 0.80
	MarketFee       = 0.05
	MinListPrice    = 0.10
	MaxListPrice    = 1000000.0
	CollectionEvery = 4 * 24 * time.Hour
	CollectionLife  = 12 * 24 * time.Hour
	SlotsPerDrop    = 8
)

// Точка отсчёта дропов: коллекция k стартует в Epoch + k*CollectionEvery.
var Epoch = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

var (
	ErrNotFound          = errors.New("nft not found")
	ErrNotOwner          = errors.New("you do not own this nft")
	ErrListed            = errors.New("nft is listed on the market")
	ErrNotListed         = errors.New("nft is not listed")
	ErrListingGone       = errors.New("listing is no longer available")
	ErrOwnListing        = errors.New("you cannot buy your own listing")
	ErrInsufficientFunds = errors.New("insufficient balance")
	ErrSoldOut           = errors.New("sold out")
	ErrInvalidPrice      = errors.New("invalid price")
	ErrDropClosed        = errors.New("this drop has ended")
	ErrWalletMissing     = errors.New("wallet not found")
)

type rarityConf struct {
	Key        string
	Lo, Hi     float64
	Yield      float64 // доля стоимости в сутки
	Supply     int
	GameChance float64
	StockLo    int
	StockHi    int
	Odds       int // вес в Mystery Capsule, из 1000
}

// Доходность растёт с редкостью, но остаётся ниже майнинга и без энергии.
var Rarities = []rarityConf{
	{"common", 3, 9, 0.0040, 400, 1.00, 6, 12, 460},
	{"uncommon", 8, 24, 0.0048, 200, 0.90, 3, 8, 300},
	{"rare", 22, 70, 0.0056, 80, 0.70, 2, 4, 150},
	{"epic", 60, 200, 0.0064, 30, 0.45, 1, 2, 65},
	{"mythic", 180, 650, 0.0075, 10, 0.25, 1, 1, 22},
	{"legendary", 600, 2400, 0.0090, 3, 0.08, 1, 1, 3},
}

// Слоты дропа: две простые, две необычные и по одной старшей редкости.
var dropSlots = []string{"common", "common", "uncommon", "uncommon", "rare", "epic", "mythic", "legendary"}

func rarityOf(key string) (rarityConf, bool) {
	for _, r := range Rarities {
		if r.Key == key {
			return r, true
		}
	}
	return rarityConf{}, false
}

func ValidRarity(key string) bool {
	_, ok := rarityOf(key)
	return ok
}

// ItemValue — оценочная стоимость копии: ранние номера ценятся выше.
func ItemValue(base float64, serial int) float64 {
	mult := 1.0
	switch {
	case serial == 1:
		mult = 1.5
	case serial <= 5:
		mult = 1.15
	}
	return round2(base * mult)
}

func InstantPrice(value float64) float64 {
	return math.Floor(value*InstantRate*100) / 100
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

type theme struct {
	Key     string
	Names   []string
	Tagline string
	Words   []string
	Sprites []string
}

var themes = []theme{
	{"frost", []string{"Frost Pals", "Polar Club", "Snowbound", "Winter Den"}, "Cold paws, warm hearts.",
		[]string{"Icy", "Glacier", "Snow", "Frost", "Aurora", "Polar", "Crystal", "Sleet"},
		[]string{"penguin", "owl", "bear", "fox", "gem", "cat"}},
	{"neon", []string{"Neon Arcade", "Glitch Gang", "Synth Club", "Night Shift"}, "Plugged in after midnight.",
		[]string{"Neon", "Glitch", "Pixel", "Synth", "Laser", "Volt", "Byte", "Retro"},
		[]string{"robot", "ghost", "cat", "slime", "rocket", "jelly"}},
	{"ember", []string{"Ember Kin", "Cinder Crew", "Hearthborn", "Ash & Flame"}, "Born in the glow of the forge.",
		[]string{"Ember", "Cinder", "Blaze", "Ash", "Magma", "Spark", "Flare", "Scorch"},
		[]string{"fox", "slime", "gem", "ghost", "cat", "bear"}},
	{"bloom", []string{"Bloom Grove", "Moss Folk", "Petal Society", "Greenhouse"}, "Grown slowly, kept forever.",
		[]string{"Moss", "Fern", "Petal", "Clover", "Willow", "Sprout", "Thorn", "Meadow"},
		[]string{"mushroom", "frog", "owl", "cactus", "bear", "slime"}},
	{"tide", []string{"Tidepool", "Deep Current", "Coral Club", "Lagoon"}, "Everything drifts back to the sea.",
		[]string{"Coral", "Reef", "Tidal", "Pearl", "Abyss", "Brine", "Lagoon", "Surf"},
		[]string{"jelly", "frog", "slime", "penguin", "gem", "ghost"}},
	{"cosmos", []string{"Cosmo Crew", "Orbiters", "Stardust", "Zero-G Club"}, "Collected somewhere past the moon.",
		[]string{"Nova", "Orbit", "Comet", "Lunar", "Nebula", "Quasar", "Astro", "Void"},
		[]string{"planet", "robot", "ghost", "rocket", "cat", "gem"}},
	{"dune", []string{"Dune Drifters", "Mirage", "Sandglass", "Oasis"}, "Found where the heat bends the air.",
		[]string{"Dune", "Mirage", "Sand", "Sahara", "Ochre", "Canyon", "Dust", "Sun"},
		[]string{"cactus", "fox", "cat", "gem", "owl", "robot"}},
	{"candy", []string{"Candy Pop", "Sugar Rush", "Gummy Gang", "Sprinkle Club"}, "Sweet, small and impossible to share.",
		[]string{"Gummy", "Sugar", "Taffy", "Mochi", "Berry", "Fizzy", "Cocoa", "Jelly"},
		[]string{"slime", "bear", "mushroom", "cat", "ghost", "jelly"}},
}

var spriteNouns = map[string][]string{
	"cat":      {"Cat", "Kitty"},
	"owl":      {"Owl", "Hoot"},
	"fox":      {"Fox", "Kit"},
	"bear":     {"Bear", "Cub"},
	"ghost":    {"Ghost", "Spook"},
	"robot":    {"Bot", "Droid"},
	"mushroom": {"Shroom", "Cap"},
	"gem":      {"Gem", "Shard"},
	"planet":   {"Planet", "World"},
	"penguin":  {"Penguin", "Pingu"},
	"frog":     {"Frog", "Toad"},
	"slime":    {"Slime", "Blob"},
	"cactus":   {"Cactus", "Prick"},
	"rocket":   {"Rocket", "Shuttle"},
	"jelly":    {"Jelly", "Medusa"},
}

type packSlot struct {
	Rarity string `json:"rarity"`
	Count  int    `json:"count"`
}

type packDef struct {
	Code    string
	Name    string
	Kind    string // random | fixed | mystery
	Caption string
	Slots   []packSlot
	Stock   int
	Markup  float64
}

// Каждый дроп продаёт одинаковый набор паков: случайные, известные и капсулу с шансами.
var packDefs = []packDef{
	{"sampler", "Sampler", "random", "3 random Common", []packSlot{{"common", 3}}, 400, 1.2},
	{"trio", "Trio", "random", "3 random Uncommon", []packSlot{{"uncommon", 3}}, 200, 1.2},
	{"collector", "Collector Box", "fixed", "Common · Uncommon · Rare", []packSlot{{"common", 1}, {"uncommon", 1}, {"rare", 1}}, 120, 1.18},
	{"vault", "Vault Box", "fixed", "Uncommon · Rare · Epic", []packSlot{{"uncommon", 1}, {"rare", 1}, {"epic", 1}}, 40, 1.15},
	{"capsule", "Mystery Capsule", "mystery", "1 NFT · any rarity", []packSlot{{"", 1}}, 250, 1.2},
}

func packDefByCode(code string) (packDef, bool) {
	for _, p := range packDefs {
		if p.Code == code {
			return p, true
		}
	}
	return packDef{}, false
}
