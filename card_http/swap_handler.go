package card_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/card_history_sql"
	"xwallet-server/card_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type swapRequest struct {
	FromAsset  string  `json:"fromAsset"`
	ToAsset    string  `json:"toAsset"`
	FromAmount float64 `json:"fromAmount"`
}

type swapResponse struct {
	FromAsset  string  `json:"fromAsset"`
	ToAsset    string  `json:"toAsset"`
	FromAmount float64 `json:"fromAmount"`
	ToAmount   float64 `json:"toAmount"`
	Rate       float64 `json:"rate"`
}

func isValidAsset(asset string) bool {
	return asset == "USDT" || card_sql.IsSupportedCoin(asset)
}

func SwapHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req swapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !isValidAsset(req.FromAsset) || !isValidAsset(req.ToAsset) {
			http.Error(w, "unsupported asset", http.StatusBadRequest)
			return
		}
		if req.FromAsset == req.ToAsset {
			http.Error(w, "cannot swap the same asset", http.StatusBadRequest)
			return
		}
		if req.FromAmount <= 0 {
			http.Error(w, "amount must be positive", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		coinsToFetch := []string{}
		if req.FromAsset != "USDT" {
			coinsToFetch = append(coinsToFetch, req.FromAsset)
		}
		if req.ToAsset != "USDT" {
			coinsToFetch = append(coinsToFetch, req.ToAsset)
		}

		var prices map[string]float64
		if len(coinsToFetch) > 0 {
			prices, err = fetchLivePrices(coinsToFetch)
			if err != nil {
				http.Error(w, "could not fetch live prices", http.StatusInternalServerError)
				return
			}
		}

		usdValue := req.FromAmount
		if req.FromAsset != "USDT" {
			price := prices[req.FromAsset]
			if price <= 0 {
				http.Error(w, "could not price source asset", http.StatusInternalServerError)
				return
			}
			usdValue = req.FromAmount * price
		}

		toAmount := usdValue
		if req.ToAsset != "USDT" {
			toPrice := prices[req.ToAsset]
			if toPrice <= 0 {
				http.Error(w, "could not price destination asset", http.StatusInternalServerError)
				return
			}
			toAmount = usdValue / toPrice
		}

		moveErr := card_sql.ExecuteAssetMove(r.Context(), pool, userID, req.FromAsset, req.ToAsset, req.FromAmount, toAmount)
		if moveErr != nil {
			if moveErr == card_sql.ErrInsufficientAssetBalance {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "swap failed", http.StatusInternalServerError)
			return
		}

		swapPrice := 0.0
		if toAmount > 0 {
			swapPrice = usdValue / req.FromAmount
		}
		card_history_sql.InsertEntryPool(r.Context(), pool, userID, "swap", req.FromAsset, req.ToAsset, req.FromAmount, toAmount, swapPrice)

		rate := 0.0

		if req.FromAmount > 0 {
			rate = toAmount / req.FromAmount
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(swapResponse{
			FromAsset: req.FromAsset, ToAsset: req.ToAsset,
			FromAmount: req.FromAmount, ToAmount: toAmount, Rate: rate,
		})
	}
}
