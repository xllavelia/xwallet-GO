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
			{Min: 0.3, Max: 0.6, Weight: 220},
			{Min: 0.7, Max: 1.0, Weight: 180},
			{Min: 1.1, Max: 1.9, Weight: 255},
			{Min: 2.5, Max: 4.0, Weight: 130},
			{Min: 5.0, Max: 5.0, Weight: 10},
		},
	},
	{
		Rarity:      "signature",
		DisplayName: "Signature",
		Price:       50,
		TicketCount: 20,
		Tiers: []ValueTier{
			{Min: 0.5, Max: 0.9, Weight: 200},
			{Min: 1.0, Max: 1.6, Weight: 340},
			{Min: 1.8, Max: 3.5, Weight: 280},
			{Min: 4.0, Max: 7.0, Weight: 140},
			{Min: 8.0, Max: 9.5, Weight: 60},
			{Min: 10.0, Max: 10.0, Weight: 7},
		},
	},
	{
		Rarity:      "sovereign",
		DisplayName: "Sovereign",
		Price:       100,
		TicketCount: 20,
		Tiers: []ValueTier{
			{Min: 2.0, Max: 2.9, Weight: 300},
			{Min: 3.0, Max: 4.5, Weight: 330},
			{Min: 4.6, Max: 7.0, Weight: 210},
			{Min: 7.5, Max: 12.0, Weight: 120},
			{Min: 13.0, Max: 20.0, Weight: 30},
			{Min: 25.0, Max: 25.0, Weight: 5},
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
