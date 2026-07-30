package positions_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/positions_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type openPositionRequest struct {
	Coin            string   `json:"coin"`
	Type            string   `json:"type"`
	EntryPrice      float64  `json:"entryPrice"`
	Leverage        int      `json:"leverage"`
	Amount          float64  `json:"amount"`
	AutoClose       bool     `json:"autoClose"`
	AutoCloseTarget *float64 `json:"autoCloseTarget"`
}

type positionResponse struct {
	ID                int      `json:"id"`
	TradeID           string   `json:"tradeId"`
	Coin              string   `json:"coin"`
	Type              string   `json:"type"`
	EntryPrice        float64  `json:"entryPrice"`
	Leverage          int      `json:"leverage"`
	Amount            float64  `json:"amount"`
	Margin            float64  `json:"margin"`
	Fees              float64  `json:"fees"`
	FeesPaidByVoucher bool     `json:"feesPaidByVoucher"`
	LiqPrice          float64  `json:"liqPrice"`
	AutoClose         bool     `json:"autoClose"`
	AutoCloseTarget   *float64 `json:"autoCloseTarget"`
	OpenedAt          string   `json:"openedAt"`
}

func OpenPositionHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req openPositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Type != "long" && req.Type != "short" {
			http.Error(w, "type must be 'long' or 'short'", http.StatusBadRequest)
			return
		}
		if req.Amount <= 0 {
			http.Error(w, "amount must be positive", http.StatusBadRequest)
			return
		}
		if req.Leverage <= 0 || req.Leverage > 200 {
			http.Error(w, "leverage must be between 1 and 200", http.StatusBadRequest)
			return
		}
		if req.EntryPrice <= 0 {
			http.Error(w, "invalid entry price", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		balance, err := wallet_sql.GetBalanceByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not read wallet", http.StatusInternalServerError)
			return
		}

		margin := req.Amount / float64(req.Leverage)
		fees := CalcFees(req.Amount)
		totalRequired := margin + fees

		if balance < totalRequired {
			http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			return
		}

		liqPrice := CalcLiquidationPrice(req.EntryPrice, req.Leverage, req.Type)

		pos := positions_sql.Position{
			TradeID:           generateTradeID(),
			UserID:            userID,
			Coin:              req.Coin,
			Type:              req.Type,
			EntryPrice:        req.EntryPrice,
			Leverage:          req.Leverage,
			Amount:            req.Amount,
			Margin:            margin,
			Fees:              fees,
			FeesPaidByVoucher: false,
			LiqPrice:          liqPrice,
			AutoClose:         req.AutoClose,
			AutoCloseTarget:   req.AutoCloseTarget,
		}

		created, err := positions_sql.InsertPosition(r.Context(), pool, pos)
		if err != nil {
			http.Error(w, "could not open position", http.StatusInternalServerError)
			return
		}

		if err := wallet_sql.AdjustBalance(r.Context(), pool, userID, -totalRequired); err != nil {
			http.Error(w, "position opened but balance update failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(positionResponse{
			ID:                created.ID,
			TradeID:           created.TradeID,
			Coin:              created.Coin,
			Type:              created.Type,
			EntryPrice:        created.EntryPrice,
			Leverage:          created.Leverage,
			Amount:            created.Amount,
			Margin:            created.Margin,
			Fees:              created.Fees,
			FeesPaidByVoucher: created.FeesPaidByVoucher,
			LiqPrice:          created.LiqPrice,
			AutoClose:         created.AutoClose,
			AutoCloseTarget:   created.AutoCloseTarget,
			OpenedAt:          created.OpenedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}

func generateTradeID() string {
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, 6)
	for i := range out {
		out[i] = chars[randIndex(len(chars))]
	}
	return "TRD-" + string(out)
}
