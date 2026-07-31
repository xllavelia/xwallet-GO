package transfer_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/transfer_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type sendTransferRequest struct {
	RecipientPlayerID string  `json:"recipientPlayerId"`
	Amount            float64 `json:"amount"`
}

func toDetailResponse(d transfer_sql.TransferDetail) map[string]interface{} {
	return map[string]interface{}{
		"id":                   d.ID,
		"direction":            d.Direction,
		"amount":               d.Amount,
		"counterpartyUsername": d.CounterpartyUsername,
		"counterpartyPlayerId": d.CounterpartyPlayerID,
		"referenceCode":        d.ReferenceCode,
		"status":               d.Status,
		"createdAt":            d.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func SendTransferHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req sendTransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Amount <= 0 {
			http.Error(w, "amount must be positive", http.StatusBadRequest)
			return
		}

		senderID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "sender not found", http.StatusNotFound)
			return
		}

		recipientID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.RecipientPlayerID)
		if err != nil {
			http.Error(w, "recipient not found", http.StatusNotFound)
			return
		}

		if senderID == recipientID {
			http.Error(w, "cannot send to yourself", http.StatusBadRequest)
			return
		}

		transferID, err := transfer_sql.ExecuteTransfer(r.Context(), pool, senderID, recipientID, req.Amount)
		if err != nil {
			if err == transfer_sql.ErrInsufficientBalance {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "transfer failed", http.StatusInternalServerError)
			return
		}

		detail, err := transfer_sql.GetTransferDetail(r.Context(), pool, transferID, senderID)
		if err != nil {
			http.Error(w, "transfer completed but detail load failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toDetailResponse(detail))
	}
}
