package card_sql

var columnByCoin = map[string]string{
	"BTC": "btc_amount",
	"ETH": "eth_amount",
	"SOL": "sol_amount",
	"TON": "ton_amount",
}

func IsSupportedCoin(coin string) bool {
	_, ok := columnByCoin[coin]
	return ok
}
