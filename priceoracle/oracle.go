package priceoracle

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

var SupportedCoins = []string{"BTC", "ETH", "SOL", "TON"}

var yahooSymbolFor = map[string]string{
	"BTC": "BTC-USD",
	"ETH": "ETH-USD",
	"SOL": "SOL-USD",
	"TON": "TON11419-USD",
}

var mu sync.RWMutex
var cache = map[string]float64{}
var currentInterval = 20 * time.Second
var maxInterval = 5 * time.Minute

var client = &http.Client{Timeout: 8 * time.Second}

func Start() {
	go func() {
		for {
			ok := tickAll()
			mu.Lock()
			if ok {
				currentInterval = 20 * time.Second
			} else if currentInterval < maxInterval {
				currentInterval *= 2
				if currentInterval > maxInterval {
					currentInterval = maxInterval
				}
			}
			wait := currentInterval
			mu.Unlock()
			time.Sleep(wait)
		}
	}()
	log.Println("price oracle started (crypto via Yahoo Finance, same scheme as stocks)")
}

type chartMetaResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

func fetchOne(coin string, yahooSymbol string) (float64, bool) {
	url := "https://query1.finance.yahoo.com/v8/finance/chart/" + yahooSymbol + "?range=1d&interval=1d"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		log.Println("price oracle: non-200 for", coin, resp.StatusCode, string(body))
		return 0, false
	}

	var parsed chartMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, false
	}
	if len(parsed.Chart.Result) == 0 {
		return 0, false
	}
	price := parsed.Chart.Result[0].Meta.RegularMarketPrice
	if price <= 0 {
		return 0, false
	}
	return price, true
}

func tickAll() bool {
	anySuccess := false
	for _, coin := range SupportedCoins {
		yahooSym, ok := yahooSymbolFor[coin]
		if !ok {
			continue
		}
		price, ok := fetchOne(coin, yahooSym)
		if ok {
			mu.Lock()
			cache[coin] = price
			mu.Unlock()
			anySuccess = true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return anySuccess
}

func Get(coin string) (float64, bool) {
	mu.RLock()
	defer mu.RUnlock()
	v, ok := cache[coin]
	return v, ok
}

func GetAll(coins []string) map[string]float64 {
	mu.RLock()
	defer mu.RUnlock()
	out := map[string]float64{}
	for _, c := range coins {
		if v, ok := cache[c]; ok {
			out[c] = v
		}
	}
	return out
}