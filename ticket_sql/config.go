package ticket_sql

type ValueTier struct {
	Min    float64
	Max    float64
	Weight int
}

type PackConfig struct {
	Rarity      string
	DisplayName string
	Price       float64
	TicketCount int
	Tiers       []ValueTier
}

var Packs = []PackConfig{
	{
		Rarity:      "essential",
		DisplayName: "Essential",
		Price:       20,
		TicketCount: 20,
		Tiers: []ValueTier{
			{Min: 0.42, Max: 0.84, Weight: 247},
			{Min: 0.98, Max: 1.40, Weight: 163},
			{Min: 1.55, Max: 2.68, Weight: 291},
			{Min: 3.55, Max: 5.65, Weight: 116},
			{Min: 7.10, Max: 7.10, Weight: 13},
		},
	},

	{
		Rarity:      "signature",
		DisplayName: "Signature",
		Price:       50,
		TicketCount: 20,
		Tiers: []ValueTier{
			{Min: 0.70, Max: 1.27, Weight: 218},
			{Min: 1.40, Max: 2.25, Weight: 317},
			{Min: 2.55, Max: 4.95, Weight: 264},
			{Min: 5.65, Max: 9.85, Weight: 127},
			{Min: 11.30, Max: 13.40, Weight: 51},
			{Min: 14.10, Max: 14.10, Weight: 8},
		},
	},

	{
		Rarity:      "sovereign",
		DisplayName: "Sovereign",
		Price:       100,
		TicketCount: 20,
		Tiers: []ValueTier{
			{Min: 2.80, Max: 4.10, Weight: 273},
			{Min: 4.25, Max: 6.35, Weight: 354},
			{Min: 6.50, Max: 9.90, Weight: 187},
			{Min: 10.60, Max: 16.95, Weight: 139},
			{Min: 18.35, Max: 28.20, Weight: 41},
			{Min: 35.25, Max: 50, Weight: 6},
		},
	},
}

func PackByRarity(rarity string) (PackConfig, bool) {
	for _, p := range Packs {
		if p.Rarity == rarity {
			return p, true
		}
	}
	return PackConfig{}, false
}
