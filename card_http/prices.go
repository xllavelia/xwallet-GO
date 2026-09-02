package card_http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
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

var (
	priceCacheMu sync.Mutex
	priceCache   = make(map[string]float64)
	priceCacheAt time.Time

	fetchMu sync.Mutex
)

const priceCacheTTL = 10 * time.Second

func fetchLivePrices(coins []string) (map[string]float64, error) {
	// Не допускаем несколько одновременных запросов к Binance.
	fetchMu.Lock()
	defer fetchMu.Unlock()

	// Возвращаем кэш, если он ещё свежий.
	priceCacheMu.Lock()
	if time.Since(priceCacheAt) < priceCacheTTL && len(priceCache) > 0 {
		result := make(map[string]float64, len(coins))

		for _, coin := range coins {
			if price, ok := priceCache[coin]; ok {
				result[coin] = price
			}
		}

		priceCacheMu.Unlock()
		return result, nil
	}
	priceCacheMu.Unlock()

	symbols := make([]string, len(coins))
	for i, c := range coins {
		symbols[i] = c + "USDT"
	}

	symbolsJSON, err := json.Marshal(symbols)
	if err != nil {
		return nil, err
	}

	url := "https://api.binance.com/api/v3/ticker/price?symbols=" +
		string(symbolsJSON)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := binanceHTTPClient.Do(req)
	if err != nil {
		log.Println("binance price fetch: request failed:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Binance rate limit.
	if resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusTeapot {

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))

		retryAfter := resp.Header.Get("Retry-After")

		log.Printf(
			"binance price fetch: rate limited status=%d retry-after=%s body=%s",
			resp.StatusCode,
			retryAfter,
			string(body),
		)

		return nil, fmt.Errorf(
			"%w: Binance rate limit status=%d retry-after=%s",
			errPriceFetchFailed,
			resp.StatusCode,
			retryAfter,
		)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))

		log.Println(
			"binance price fetch: non-200 status",
			resp.StatusCode,
			"body:",
			string(body),
		)

		return nil, errPriceFetchFailed
	}

	var tickers []binanceTicker

	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
	}

	prices := make(map[string]float64, len(tickers))

	for _, t := range tickers {
		if len(t.Symbol) <= 4 {
			continue
		}

		coin := t.Symbol[:len(t.Symbol)-4]

		price, err := strconv.ParseFloat(t.Price, 64)
		if err != nil {
			log.Printf(
				"binance price fetch: invalid price symbol=%s price=%q",
				t.Symbol,
				t.Price,
			)
			continue
		}

		prices[coin] = price
	}

	// Обновляем кэш.
	priceCacheMu.Lock()
	priceCache = prices
	priceCacheAt = time.Now()
	priceCacheMu.Unlock()

	return prices, nil
}
