package commodities_sql

type CommodityInfo struct {
	Symbol string
	Name   string
	Unit   string
	Color  string
}

var Catalog = []CommodityInfo{
	{"GC=F", "Gold", "oz", "#f0b90b"},
	{"SI=F", "Silver", "oz", "#c7c7c7"},
	{"CL=F", "Crude Oil (WTI)", "bbl", "#3a3a3a"},
	{"BZ=F", "Brent Crude", "bbl", "#5a5a5a"},
	{"NG=F", "Natural Gas", "MMBtu", "#4fa8e0"},
	{"HG=F", "Copper", "lb", "#c87f4a"},
	{"PL=F", "Platinum", "oz", "#8a99a8"},
	{"ZC=F", "Corn", "bu", "#e0b84f"},
	{"ZW=F", "Wheat", "bu", "#d4a94a"},
	{"KC=F", "Coffee", "lb", "#6f4e37"},
}

func IsValidSymbol(symbol string) bool {
	for _, c := range Catalog {
		if c.Symbol == symbol {
			return true
		}
	}
	return false
}

func GetInfo(symbol string) (CommodityInfo, bool) {
	for _, c := range Catalog {
		if c.Symbol == symbol {
			return c, true
		}
	}
	return CommodityInfo{}, false
}
