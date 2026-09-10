package p2p_sql

import (
	"xwallet-server/priceoracle"
	"xwallet-server/stocks_sql"
)

func ValidateAsset(assetClass string, asset string) bool {
	switch assetClass {
	case "lavx":
		return asset == "LAVX"
	case "crypto":
		for _, c := range priceoracle.SupportedCoins {
			if c == asset {
				return true
			}
		}
		return false
	case "stock":
		return stocks_sql.IsValidSymbol(asset)
	}
	return false
}
