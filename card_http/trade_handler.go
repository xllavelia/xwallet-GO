package card_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/card_history_sql"
	"xwallet-server/card_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tradeRequest struct {
	Coin      string  `json:"coin"`
	UsdAmount float64 `json:"usdAmount"`
	Direction string  `json:"direction"`
}
type tradeResponse struct {
	Coin       string  `json:"coin"`
	Direction  string  `json:"direction"`
	UsdAmount  float64 `json:"usdAmount"`
	CoinAmount float64 `json:"coinAmount"`
	Price      float64 `json:"price"`
	XpAwarded  int     `json:"xpAwarded"`
}

func TradeHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !card_sql.IsSupportedCoin(req.Coin) {
			http.Error(w, "unsupported coin", http.StatusBadRequest)
			return
		}
		if req.UsdAmount <= 0 {
			http.Error(w, "amount must be positive", http.StatusBadRequest)
			return
		}
		if req.Direction != "buy" && req.Direction != "sell" {
			http.Error(w, "direction must be 'buy' or 'sell'", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		prices, err := fetchLivePrices([]string{req.Coin})
		if err != nil || prices[req.Coin] <= 0 {
			http.Error(w, "could not fetch live price", http.StatusInternalServerError)
			return
		}
		price := prices[req.Coin]
		coinAmount := req.UsdAmount / price

		var moveErr error
		if req.Direction == "buy" {
			moveErr = card_sql.ExecuteAssetMove(r.Context(), pool, userID, "USDT", req.Coin, req.UsdAmount, coinAmount)
		} else {
			moveErr = card_sql.ExecuteAssetMove(r.Context(), pool, userID, req.Coin, "USDT", coinAmount, req.UsdAmount)
		}
		if moveErr != nil {
			if moveErr == card_sql.ErrInsufficientAssetBalance {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "trade failed", http.StatusInternalServerError)
			return
		}

		var xpAwarded int
		if req.Direction == "buy" {
			xpAwarded = battlepass_sql.AwardCardBuyXP(r.Context(), pool, userID, req.UsdAmount)
			card_history_sql.InsertEntryPool(r.Context(), pool, userID, "buy", "USDT", req.Coin, req.UsdAmount, coinAmount, price, xpAwarded)
		} else {
			xpAwarded = battlepass_sql.AwardCardSellXP(r.Context(), pool, userID, req.UsdAmount)
			card_history_sql.InsertEntryPool(r.Context(), pool, userID, "sell", req.Coin, "USDT", coinAmount, req.UsdAmount, price, xpAwarded)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tradeResponse{
			Coin: req.Coin, Direction: req.Direction, UsdAmount: req.UsdAmount, CoinAmount: coinAmount, Price: price, XpAwarded: xpAwarded,
		})
	}
}
