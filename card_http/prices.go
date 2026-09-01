package card_http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"
)

type binanceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

var errPriceFetchFailed = errors.New("could not fetch live prices from Binance")

var binanceHTTPClient = &http.Client{
	Timeout: 8 * time.Second,
}

func fetchLivePrices(coins []string) (map[string]float64, error) {
	symbols := make([]string, len(coins))
	for i, c := range coins {
		symbols[i] = c + "USDT"
	}
	symbolsJSON, _ := json.Marshal(symbols)
	url := "https://api.binance.com/api/v3/ticker/price?symbols=" + string(symbolsJSON)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := binanceHTTPClient.Do(req)
	if err != nil {
		log.Println("binance price fetch: request failed:", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		log.Println("binance price fetch: non-200 status", resp.StatusCode, "body:", string(body))
		return nil, errPriceFetchFailed
	}

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