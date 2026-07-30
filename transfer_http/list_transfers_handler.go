package transfer_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/transfer_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type transferListItem struct {
	ID           int     `json:"id"`
	Direction    string  `json:"direction"` // "send" | "receive"
	Counterparty string  `json:"counterparty"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"createdAt"`
}

func ListTransfersHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		transfers, err := transfer_sql.GetTransfersByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load transfers", http.StatusInternalServerError)
			return
		}

		items := make([]transferListItem, 0, len(transfers))
		for _, t := range transfers {
			direction := "receive"
			counterparty := t.SenderName
			if t.SenderID == userID {
				direction = "send"
				counterparty = t.RecipientName
			}
			items = append(items, transferListItem{
				ID: t.ID, Direction: direction, Counterparty: counterparty,
				Amount: t.Amount, Status: t.Status, CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}
