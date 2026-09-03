package bankcards_sql

type TierConfig struct {
	ID                   string
	Name                 string
	OpenPriceUsd         float64
	CashbackPercent      float64
	CashbackPercentPrime float64
	FeeReductionPoints   float64
	FeeFullyWaived       bool
	LavxPerMonth         float64
	XpBonusPercent       float64
	TransferFeePercent   float64
}

var BaseTransferFeePercent = 1.0

var Tiers = map[string]TierConfig{
	"standard": {ID: "standard", Name: "Standard", OpenPriceUsd: 0, TransferFeePercent: 1},
	"classico": {ID: "classico", Name: "Classico", OpenPriceUsd: 200, CashbackPercent: 1, FeeReductionPoints: 1, LavxPerMonth: 5, XpBonusPercent: 10, TransferFeePercent: 7},
	"cobalt":   {ID: "cobalt", Name: "Cobalt", OpenPriceUsd: 500, CashbackPercent: 3, FeeReductionPoints: 3, LavxPerMonth: 10, XpBonusPercent: 20, TransferFeePercent: 5},
	"astro":    {ID: "astro", Name: "Astro", OpenPriceUsd: 700, CashbackPercent: 5, FeeReductionPoints: 5, LavxPerMonth: 15, XpBonusPercent: 30, TransferFeePercent: 3},
	"saint":    {ID: "saint", Name: "Saint", OpenPriceUsd: 1500, CashbackPercent: 10, CashbackPercentPrime: 15, FeeFullyWaived: true, LavxPerMonth: 20, XpBonusPercent: 50, TransferFeePercent: 0},
}

var TierOrder = []string{"standard", "classico", "cobalt", "astro", "saint"}
var MaxCardsPerUser = 3
