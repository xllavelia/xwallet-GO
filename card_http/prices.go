package card_http

import (
	"encoding/json"
	"net/http"
)

type binanceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

func fetchLivePrices(coins []string) (map[string]float64, error) {
	symbols := make([]string, len(coins))
	for i, c := range coins {
		symbols[i] = c + "USDT"
	}
	symbolsJSON, _ := json.Marshal(symbols)
	url := "https://api.binance.com/api/v3/ticker/price?symbols=" + string(symbolsJSON)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tickers []binanceTicker
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
	}

	prices := make(map[string]float64)
	for _, t := range tickers {
		coin := t.Symbol[:len(t.Symbol)-4]
		var price float64
		json.Unmarshal([]byte(t.Price), &price)
		prices[coin] = price
	}

	return prices, nil
}
