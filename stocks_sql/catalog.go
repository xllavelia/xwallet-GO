package stocks_sql

type StockInfo struct {
	Symbol string
	Name   string
	Color  string
}

var Catalog = []StockInfo{
	{"AAPL", "Apple", "#a3aaae"},
	{"MSFT", "Microsoft", "#00a4ef"},
	{"GOOGL", "Alphabet", "#4285f4"},
	{"AMZN", "Amazon", "#ff9900"},
	{"NVDA", "Nvidia", "#76b900"},
	{"META", "Meta", "#0866ff"},
	{"TSLA", "Tesla", "#e82127"},
	{"AVGO", "Broadcom", "#cc092f"},
	{"ORCL", "Oracle", "#f80000"},
	{"CRM", "Salesforce", "#00a1e0"},
	{"ADBE", "Adobe", "#ff0000"},
	{"NFLX", "Netflix", "#e50914"},
	{"AMD", "AMD", "#ed1c24"},
	{"INTC", "Intel", "#0071c5"},
	{"CSCO", "Cisco", "#1ba0d7"},
	{"IBM", "IBM", "#054ada"},
	{"UBER", "Uber", "#dddddd"},
	{"SHOP", "Shopify", "#95bf47"},
	{"SPOT", "Spotify", "#1db954"},
	{"PYPL", "PayPal", "#0f9ee8"},
}

func IsValidSymbol(symbol string) bool {
	for _, s := range Catalog {
		if s.Symbol == symbol {
			return true
		}
	}
	return false
}

func GetInfo(symbol string) (StockInfo, bool) {
	for _, s := range Catalog {
		if s.Symbol == symbol {
			return s, true
		}
	}
	return StockInfo{}, false
}
