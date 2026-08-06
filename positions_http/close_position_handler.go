package positions_http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/positions_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type closePositionRequest struct {
	ClosePrice float64 `json:"closePrice"`
}
type closePositionResponse struct {
	XpAwarded int `json:"xpAwarded"`
}

func ClosePositionHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid position id", http.StatusBadRequest)
			return
		}
		var req closePositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.ClosePrice <= 0 {
			http.Error(w, "invalid close price", http.StatusBadRequest)
			return
		}

		pos, err := positions_sql.GetPositionByID(r.Context(), pool, id)
		if err != nil {
			http.Error(w, "position not found", http.StatusNotFound)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil || pos.UserID != userID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if pos.Status != "open" {
			http.Error(w, "position already closed", http.StatusConflict)
			return
		}

		pnl := CalcPnl(pos.Margin, pos.Leverage, pos.EntryPrice, req.ClosePrice, pos.Type)
		pnlPercent := CalcPnlPercent(pnl, pos.Margin)
		result := "win"
		if pnl < 0 {
			result = "loss"
		}

		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "could not start transaction", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		if err := positions_sql.ClosePositionTx(r.Context(), tx, id, req.ClosePrice, pnl, pnlPercent, result); err != nil {
			http.Error(w, "could not close position", http.StatusInternalServerError)
			return
		}
		if err := wallet_sql.AdjustBalanceTx(r.Context(), tx, userID, pos.Margin+pnl); err != nil {
			http.Error(w, "could not update balance", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "could not finalize close", http.StatusInternalServerError)
			return
		}

		xpAwarded := battlepass_sql.AwardTradeXP(r.Context(), pool, userID, pnl)
		if xpAwarded > 0 {
			positions_sql.SetXpAwarded(r.Context(), pool, id, xpAwarded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(closePositionResponse{XpAwarded: xpAwarded})
	}
}
