package positions_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/positions_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type openTimeTradeRequest struct {
	Coin            string  `json:"coin"`
	Type            string  `json:"type"`
	EntryPrice      float64 `json:"entryPrice"`
	Amount          float64 `json:"amount"`
	DurationSeconds int     `json:"durationSeconds"`
}

type timeTradeResponse struct {
	ID               int     `json:"id"`
	TradeID          string  `json:"tradeId"`
	Coin             string  `json:"coin"`
	Type             string  `json:"type"`
	EntryPrice       float64 `json:"entryPrice"`
	Amount           float64 `json:"amount"`
	DurationSeconds  int     `json:"durationSeconds"`
	PayoutMultiplier float64 `json:"payoutMultiplier"`
	ExpiresAt        string  `json:"expiresAt"`
	OpenedAt         string  `json:"openedAt"`
}

func OpenTimeTradeHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req openTimeTradeRequest
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
		if req.EntryPrice <= 0 {
			http.Error(w, "invalid entry price", http.StatusBadRequest)
			return
		}
		if req.DurationSeconds < positions_sql.MinTimeTradeSeconds || req.DurationSeconds > positions_sql.MaxTimeTradeSeconds {
			http.Error(w, "duration must be between 5 minutes and 72 hours", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		fundingSource, err := bankcards_sql.ResolveFundingSource(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not resolve funding source", http.StatusInternalServerError)
			return
		}

		multiplier := positions_sql.PayoutMultiplierForDuration(req.DurationSeconds)
		expiresAt := time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)

		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "could not start transaction", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		if err := bankcards_sql.AdjustFundingBalance(r.Context(), tx, fundingSource, -req.Amount); err != nil {
			if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "could not update balance", http.StatusInternalServerError)
			return
		}

		pos := positions_sql.Position{
			TradeID: generateTradeID(), UserID: userID, Coin: req.Coin, Type: req.Type,
			EntryPrice: req.EntryPrice, Leverage: 1, Amount: req.Amount, Margin: req.Amount,
			Fees: 0, FeesPaidByVoucher: false, LiqPrice: req.EntryPrice,
			AutoClose: false, AutoCloseTarget: nil,
			FundingKind: fundingSource.Kind, FundingCardID: fundingCardIDPtr(fundingSource),
			TradeMode: "time", ExpiresAt: &expiresAt, PayoutMultiplier: &multiplier,
		}

		created, err := positions_sql.InsertPositionTx(r.Context(), tx, pos)
		if err != nil {
			http.Error(w, "could not open time trade", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "could not finalize trade", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(timeTradeResponse{
			ID: created.ID, TradeID: created.TradeID, Coin: created.Coin, Type: created.Type,
			EntryPrice: created.EntryPrice, Amount: created.Amount, DurationSeconds: req.DurationSeconds,
			PayoutMultiplier: multiplier, ExpiresAt: expiresAt.Format("2006-01-02T15:04:05Z"),
			OpenedAt: created.OpenedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}
