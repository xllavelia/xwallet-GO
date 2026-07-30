package positions_http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"xwallet-server/positions_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type binanceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

func fetchPrices(coins []string) (map[string]float64, error) {
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
		coin := t.Symbol[:len(t.Symbol)-4] // отрезаем "USDT"
		var price float64
		json.Unmarshal([]byte(`"`+t.Price+`"`), new(string))
		price = parsePrice(t.Price)
		prices[coin] = price
	}

	return prices, nil
}

func parsePrice(s string) float64 {
	var f float64
	json.Unmarshal([]byte(s), &f)
	return f
}

func StartLiquidationWorker(pool *pgxpool.Pool) {
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		for range ticker.C {
			runLiquidationCheck(pool)
		}
	}()
	log.Println("liquidation worker started (every 15s)")
}

func runLiquidationCheck(pool *pgxpool.Pool) {
	ctx := context.Background()

	positions, err := positions_sql.GetAllOpenPositions(ctx, pool)
	if err != nil || len(positions) == 0 {
		return
	}

	coinSet := map[string]bool{}
	for _, p := range positions {
		coinSet[p.Coin] = true
	}
	coins := make([]string, 0, len(coinSet))
	for c := range coinSet {
		coins = append(coins, c)
	}

	prices, err := fetchPrices(coins)
	if err != nil {
		log.Println("liquidation worker: could not fetch prices:", err)
		return
	}

	for _, p := range positions {
		currentPrice, hasPrice := prices[p.Coin]
		if !hasPrice || currentPrice <= 0 {
			continue
		}

		shouldLiquidate := false
		if p.Type == "long" && currentPrice <= p.LiqPrice {
			shouldLiquidate = true
		}
		if p.Type == "short" && currentPrice >= p.LiqPrice {
			shouldLiquidate = true
		}

		pnl := CalcPnl(p.Margin, p.Leverage, p.EntryPrice, currentPrice, p.Type)
		pnlPercent := CalcPnlPercent(pnl, p.Margin)

		shouldAutoClose := p.AutoClose && p.AutoCloseTarget != nil && pnlPercent >= *p.AutoCloseTarget

		if !shouldLiquidate && !shouldAutoClose {
			continue
		}

		closePrice := currentPrice
		if shouldLiquidate {
			closePrice = p.LiqPrice
		}
		result := "win"
		if pnl < 0 {
			result = "loss"
		}

		if err := positions_sql.ClosePosition(ctx, pool, p.ID, closePrice, pnl, pnlPercent, result); err != nil {
			log.Println("liquidation worker: could not close position", p.ID, err)
			continue
		}
		if err := wallet_sql.AdjustBalance(ctx, pool, p.UserID, p.Margin+pnl); err != nil {
			log.Println("liquidation worker: could not adjust balance for position", p.ID, err)
			continue
		}

		if shouldLiquidate {
			log.Println("liquidated position", p.TradeID)
		} else {
			log.Println("auto-closed position", p.TradeID, "at target profit")
		}
	}
}
