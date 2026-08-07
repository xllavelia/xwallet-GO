package prime_sql

type TierConfig struct {
	ID               string
	Name             string
	MonthlyPriceLavx float64
	AnnualPriceLavx  float64
	FeeRatePercent   float64
	FeeFree          bool
	MaxVoucherSlots  int
	UsdVoucherAmount float64
	FeeVoucherLimit  float64
	FeeVoucherDays   int
	RefXpVoucher     float64
}

var BaseFeeRatePercent = 1.0
var DefaultVoucherSlots = 10

var Tiers = map[string]TierConfig{
	"pro": {
		ID: "pro", Name: "Pro",
		MonthlyPriceLavx: 90, AnnualPriceLavx: 70,
		FeeRatePercent: 0.8, FeeFree: false,
		MaxVoucherSlots:  10,
		UsdVoucherAmount: 25,
		FeeVoucherLimit:  50, FeeVoucherDays: 2,
		RefXpVoucher: 25,
	},
	"prime": {
		ID: "prime", Name: "Prime",
		MonthlyPriceLavx: 150, AnnualPriceLavx: 120,
		FeeRatePercent: 0.5, FeeFree: false,
		MaxVoucherSlots:  10,
		UsdVoucherAmount: 50,
		FeeVoucherLimit:  100, FeeVoucherDays: 4,
		RefXpVoucher: 50,
	},
	"star": {
		ID: "star", Name: "Star",
		MonthlyPriceLavx: 250, AnnualPriceLavx: 200,
		FeeRatePercent: 0, FeeFree: true,
		MaxVoucherSlots:  15,
		UsdVoucherAmount: 100,
		FeeVoucherLimit:  0, FeeVoucherDays: 0,
		RefXpVoucher: 100,
	},
}

func TierOrder() []string { return []string{"pro", "prime", "star"} }