package stockoracle

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type Quote struct {
	Symbol        string
	Price         float64
	PreviousClose float64
}

var mu sync.RWMutex
var cache = map[string]Quote{}
var currentInterval = 30 * time.Second
var maxInterval = 5 * time.Minute

var client = &http.Client{Timeout: 8 * time.Second}

var Symbols = []string{
	"AAPL", "MSFT", "GOOGL", "AMZN", "NVDA", "META", "TSLA", "AVGO", "ORCL", "CRM",
	"ADBE", "NFLX", "AMD", "INTC", "CSCO", "IBM", "UBER", "SHOP", "SPOT", "PYPL",
}

func Start() {
	go func() {
		for {
			ok := tickAll()
			mu.Lock()
			if ok {
				currentInterval = 30 * time.Second
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
	log.Println("stock oracle started (Yahoo Finance, staggered polling, backoff on rate limit)")
}

type chartMetaResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"previousClose"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

func fetchOne(symbol string) (Quote, bool) {
	url := "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?range=1d&interval=1d"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Quote{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return Quote{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		log.Println("stock oracle: non-200 for", symbol, resp.StatusCode, string(body))
		return Quote{}, false
	}

	var parsed chartMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Quote{}, false
	}
	if len(parsed.Chart.Result) == 0 {
		return Quote{}, false
	}
	meta := parsed.Chart.Result[0].Meta
	prevClose := meta.PreviousClose
	if prevClose == 0 {
		prevClose = meta.ChartPreviousClose
	}
	return Quote{Symbol: symbol, Price: meta.RegularMarketPrice, PreviousClose: prevClose}, true
}

func tickAll() bool {
	anySuccess := false
	for _, sym := range Symbols {
		q, ok := fetchOne(sym)
		if ok {
			mu.Lock()
			cache[sym] = q
			mu.Unlock()
			anySuccess = true
		}
		time.Sleep(350 * time.Millisecond)
	}
	return anySuccess
}

func GetAll() map[string]Quote {
	mu.RLock()
	defer mu.RUnlock()
	out := map[string]Quote{}
	for k, v := range cache {
		out[k] = v
	}
	return out
}

func Get(symbol string) (Quote, bool) {
	mu.RLock()
	defer mu.RUnlock()
	v, ok := cache[symbol]
	return v, ok
}
