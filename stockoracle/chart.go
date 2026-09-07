package stockoracle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type ChartPoint struct {
	Time  int64   `json:"time"`
	Price float64 `json:"price"`
}

var chartCacheMu sync.Mutex

type chartCacheEntry struct {
	points    []ChartPoint
	fetchedAt time.Time
}

var chartCache = map[string]chartCacheEntry{}

func FetchChart(symbol string, rangeStr string, interval string) ([]ChartPoint, error) {
	cacheKey := symbol + "|" + rangeStr + "|" + interval

	chartCacheMu.Lock()
	if entry, ok := chartCache[cacheKey]; ok && time.Since(entry.fetchedAt) < 3*time.Minute {
		chartCacheMu.Unlock()
		return entry.points, nil
	}
	chartCacheMu.Unlock()

	url := "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?range=" + rangeStr + "&interval=" + interval
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("yahoo chart non-200: %d %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, errors.New("no chart data")
	}

	res := parsed.Chart.Result[0]
	closes := res.Indicators.Quote[0].Close
	points := make([]ChartPoint, 0, len(res.Timestamp))
	for i, ts := range res.Timestamp {
		if i < len(closes) && closes[i] != nil {
			points = append(points, ChartPoint{Time: ts, Price: *closes[i]})
		}
	}

	chartCacheMu.Lock()
	chartCache[cacheKey] = chartCacheEntry{points: points, fetchedAt: time.Now()}
	chartCacheMu.Unlock()

	return points, nil
}
