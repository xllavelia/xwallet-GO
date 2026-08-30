package savings_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/savings_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type historyItem struct {
	ID        int     `json:"id"`
	EntryType string  `json:"entryType"`
	Amount    float64 `json:"amount"`
	CreatedAt string  `json:"createdAt"`
}

type savingsResponse struct {
	Balance      float64       `json:"balance"`
	InterestRate float64       `json:"interestRate"`
	TotalAccrued float64       `json:"totalAccrued"`
	History      []historyItem `json:"history"`
}

func GetSavingsHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		account, err := savings_sql.GetAccount(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load savings account", http.StatusInternalServerError)
			return
		}

		totalAccrued, err := savings_sql.GetTotalAccruedInterest(r.Context(), pool, userID)
		if err != nil {
			totalAccrued = 0
		}

		history, err := savings_sql.GetHistory(r.Context(), pool, userID)
		if err != nil {
			history = []savings_sql.HistoryEntry{}
		}

		items := make([]historyItem, 0, len(history))
		for _, h := range history {
			items = append(items, historyItem{
				ID: h.ID, EntryType: h.EntryType, Amount: h.Amount,
				CreatedAt: h.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(savingsResponse{
			Balance: account.Balance, InterestRate: savings_sql.AnnualInterestRatePercent,
			TotalAccrued: totalAccrued, History: items,
		})
	}
}
