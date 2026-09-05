package positions_http

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/battlepass_sql"
	"xwallet-server/positions_sql"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type closePositionRequest struct {
	ClosePrice float64 `json:"closePrice"`
}
type closePositionResponse struct {
	XpAwarded       int     `json:"xpAwarded"`
	CashbackAwarded float64 `json:"cashbackAwarded"`
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
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "invalid position id", http.StatusBadRequest)
			return
		}
		var req closePositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClosePrice <= 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
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

		fundingSource := bankcards_sql.FundingSource{Kind: pos.FundingKind, UserID: userID}
		if pos.FundingKind == "card" && pos.FundingCardID != nil {
			fundingSource.CardID = *pos.FundingCardID
		} else {
			fundingSource.Kind = "wallet"
		}

		cashback := 0.0
		if pnl > 0 && fundingSource.Kind == "card" {
			cardTier, tierErr := bankcards_sql.GetCardTier(r.Context(), pool, fundingSource.CardID)
			if tierErr == nil {
				if cfg, ok := bankcards_sql.Tiers[cardTier]; ok {
					rate := cfg.CashbackPercent
					if cardTier == "saint" {
						if primeSub, _ := prime_sql.GetActiveSubscription(r.Context(), pool, userID); primeSub != nil {
							rate = cfg.CashbackPercentPrime
						}
					}
					cashback = pnl * (rate / 100)
				}
			}
		}
		if cashback > 0 {
			log.Println("cashback awarded:", cashback, "for user", userID, "on card", fundingSource.CardID)
		}
		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "could not start transaction", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		if err := positions_sql.ClosePositionTx(r.Context(), tx, id, req.ClosePrice, pnl, pnlPercent, result, cashback); err != nil {
			http.Error(w, "could not close position", http.StatusInternalServerError)
			return
		}
		if err := bankcards_sql.AdjustFundingBalance(r.Context(), tx, fundingSource, pos.Margin+pnl+cashback); err != nil {
			http.Error(w, "could not update balance", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "could not finalize close", http.StatusInternalServerError)
			return
		}

		xpAwarded := battlepass_sql.AwardTradeXP(r.Context(), pool, userID, pnl, fundingSource)
		if xpAwarded > 0 {
			positions_sql.SetXpAwarded(r.Context(), pool, id, xpAwarded)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(closePositionResponse{XpAwarded: xpAwarded, CashbackAwarded: cashback})
	}
}
