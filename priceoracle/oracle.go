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

var mu sync.RWMutex
var cache = map[string]float64{}
var lastSuccessAt time.Time
var currentInterval = 10 * time.Second
var maxInterval = 5 * time.Minute

var client = &http.Client{Timeout: 8 * time.Second}

type ticker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

func Start() {
	go func() {
		for {
			ok := tick()
			mu.Lock()
			if ok {
				currentInterval = 10 * time.Second
				lastSuccessAt = time.Now()
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
	log.Println("price oracle started (crypto, single poller, backoff on rate limit)")
}

func tick() bool {
	symbols := make([]string, len(SupportedCoins))
	for i, c := range SupportedCoins {
		symbols[i] = c + "USDT"
	}
	symbolsJSON, _ := json.Marshal(symbols)
	url := "https://api.binance.com/api/v3/ticker/price?symbols=" + string(symbolsJSON)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		log.Println("price oracle: fetch failed:", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		log.Println("price oracle: non-200 status", resp.StatusCode, string(body))
		return false
	}

	var tickers []ticker
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		log.Println("price oracle: decode failed:", err)
		return false
	}

	mu.Lock()
	for _, t := range tickers {
		if len(t.Symbol) <= 4 {
			continue
		}
		coin := t.Symbol[:len(t.Symbol)-4]
		var price float64
		json.Unmarshal([]byte(t.Price), &price)
		if price > 0 {
			cache[coin] = price
		}
	}
	mu.Unlock()

	return true
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

func HasAnyData() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(cache) > 0
}
