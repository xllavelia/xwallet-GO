package card_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/card_history_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type cardHistoryItem struct {
	ID            int     `json:"id"`
	OperationType string  `json:"operationType"`
	FromAsset     string  `json:"fromAsset"`
	ToAsset       string  `json:"toAsset"`
	FromAmount    float64 `json:"fromAmount"`
	ToAmount      float64 `json:"toAmount"`
	Price         float64 `json:"price"`
	CreatedAt     string  `json:"createdAt"`
}

func toHistoryItem(e card_history_sql.CardHistoryEntry) cardHistoryItem {
	return cardHistoryItem{
		ID: e.ID, OperationType: e.OperationType, FromAsset: e.FromAsset, ToAsset: e.ToAsset,
		FromAmount: e.FromAmount, ToAmount: e.ToAmount, Price: e.Price,
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func ListCardHistoryHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		entries, err := card_history_sql.GetHistoryByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load history", http.StatusInternalServerError)
			return
		}

		items := make([]cardHistoryItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, toHistoryItem(e))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}
