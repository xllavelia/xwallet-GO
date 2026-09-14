package commodities_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/commodities_sql"
	"xwallet-server/commodityoracle"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var timeframeParams = map[string]struct{ Range, Interval string }{
	"1D": {"1d", "5m"}, "1W": {"5d", "15m"}, "1M": {"1mo", "1d"}, "3M": {"3mo", "1d"}, "1Y": {"1y", "1wk"},
}

type catalogItem struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Unit          string  `json:"unit"`
	Color         string  `json:"color"`
	Price         float64 `json:"price"`
	ChangeAmount  float64 `json:"changeAmount"`
	ChangePercent float64 `json:"changePercent"`
}

func CatalogHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		quotes := commodityoracle.GetAll()
		items := make([]catalogItem, 0, len(commodities_sql.Catalog))
		for _, c := range commodities_sql.Catalog {
			q := quotes[c.Symbol]
			changeAmount := q.Price - q.PreviousClose
			changePercent := 0.0
			if q.PreviousClose > 0 {
				changePercent = (changeAmount / q.PreviousClose) * 100
			}
			items = append(items, catalogItem{
				Symbol: c.Symbol, Name: c.Name, Unit: c.Unit, Color: c.Color,
				Price: q.Price, ChangeAmount: changeAmount, ChangePercent: changePercent,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

func ChartHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		symbol := r.URL.Query().Get("symbol")
		if !commodities_sql.IsValidSymbol(symbol) {
			http.Error(w, "invalid symbol", http.StatusBadRequest)
			return
		}
		params, ok := timeframeParams[r.URL.Query().Get("timeframe")]
		if !ok {
			params = timeframeParams["1M"]
		}
		points, err := commodityoracle.FetchChart(symbol, params.Range, params.Interval)
		if err != nil {
			http.Error(w, "could not load chart data", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(points)
	}
}

type holdingItem struct {
	Symbol               string  `json:"symbol"`
	Name                 string  `json:"name"`
	Unit                 string  `json:"unit"`
	Color                string  `json:"color"`
	Quantity             float64 `json:"quantity"`
	AvgCost              float64 `json:"avgCost"`
	CurrentPrice         float64 `json:"currentPrice"`
	CurrentValue         float64 `json:"currentValue"`
	CostBasis            float64 `json:"costBasis"`
	UnrealizedPnl        float64 `json:"unrealizedPnl"`
	UnrealizedPnlPercent float64 `json:"unrealizedPnlPercent"`
}

type portfolioResponse struct {
	Holdings                  []holdingItem `json:"holdings"`
	TotalValue                float64       `json:"totalValue"`
	TotalCostBasis            float64       `json:"totalCostBasis"`
	TotalUnrealizedPnl        float64       `json:"totalUnrealizedPnl"`
	TotalUnrealizedPnlPercent float64       `json:"totalUnrealizedPnlPercent"`
	TodayChangeAmount         float64       `json:"todayChangeAmount"`
	TodayChangePercent        float64       `json:"todayChangePercent"`
}

func PortfolioHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		holdings, err := commodities_sql.GetHoldings(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load holdings", http.StatusInternalServerError)
			return
		}
		quotes := commodityoracle.GetAll()

		items := make([]holdingItem, 0, len(holdings))
		var totalValue, totalCost, todayChangeAmount float64
		for _, h := range holdings {
			info, _ := commodities_sql.GetInfo(h.Symbol)
			q := quotes[h.Symbol]
			currentValue := h.Quantity * q.Price
			costBasis := h.Quantity * h.AvgCost
			unrealizedPnl := currentValue - costBasis
			unrealizedPct := 0.0
			if costBasis > 0 {
				unrealizedPct = (unrealizedPnl / costBasis) * 100
			}
			todayChange := (q.Price - q.PreviousClose) * h.Quantity

			items = append(items, holdingItem{
				Symbol: h.Symbol, Name: info.Name, Unit: info.Unit, Color: info.Color,
				Quantity: h.Quantity, AvgCost: h.AvgCost, CurrentPrice: q.Price,
				CurrentValue: currentValue, CostBasis: costBasis,
				UnrealizedPnl: unrealizedPnl, UnrealizedPnlPercent: unrealizedPct,
			})
			totalValue += currentValue
			totalCost += costBasis
			todayChangeAmount += todayChange
		}

		totalPnl := totalValue - totalCost
		totalPnlPct := 0.0
		if totalCost > 0 {
			totalPnlPct = (totalPnl / totalCost) * 100
		}
		yesterdayValue := totalValue - todayChangeAmount
		todayChangePct := 0.0
		if yesterdayValue > 0 {
			todayChangePct = (todayChangeAmount / yesterdayValue) * 100
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(portfolioResponse{
			Holdings: items, TotalValue: totalValue, TotalCostBasis: totalCost,
			TotalUnrealizedPnl: totalPnl, TotalUnrealizedPnlPercent: totalPnlPct,
			TodayChangeAmount: todayChangeAmount, TodayChangePercent: todayChangePct,
		})
	}
}

type tradeRequest struct {
	Symbol    string  `json:"symbol"`
	UsdAmount float64 `json:"usdAmount"`
}
type buyResponse struct {
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price"`
}
type sellResponse struct {
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price"`
	RealizedPnl float64 `json:"realizedPnl"`
}

func BuyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req tradeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UsdAmount <= 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !commodities_sql.IsValidSymbol(req.Symbol) {
			http.Error(w, "invalid symbol", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		quote, hasPrice := commodityoracle.Get(req.Symbol)
		if !hasPrice || quote.Price <= 0 {
			http.Error(w, "price not available yet, try again in a moment", http.StatusServiceUnavailable)
			return
		}
		quantity, err := commodities_sql.BuyCommodity(r.Context(), pool, userID, req.Symbol, req.UsdAmount, quote.Price)
		if err != nil {
			if err == commodities_sql.ErrInsufficientFunds {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "could not complete purchase", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buyResponse{Quantity: quantity, Price: quote.Price})
	}
}

func SellHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req tradeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UsdAmount <= 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		quote, hasPrice := commodityoracle.Get(req.Symbol)
		if !hasPrice || quote.Price <= 0 {
			http.Error(w, "price not available yet, try again in a moment", http.StatusServiceUnavailable)
			return
		}
		quantity, realizedPnl, err := commodities_sql.SellCommodity(r.Context(), pool, userID, req.Symbol, req.UsdAmount, quote.Price)
		if err != nil {
			if err == commodities_sql.ErrNoHolding {
				http.Error(w, "you don't own this commodity", http.StatusBadRequest)
				return
			}
			http.Error(w, "could not complete sale", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sellResponse{Quantity: quantity, Price: quote.Price, RealizedPnl: realizedPnl})
	}
}
