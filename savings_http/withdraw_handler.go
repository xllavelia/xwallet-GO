package savings_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/savings_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func WithdrawHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req amountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		err = savings_sql.Withdraw(r.Context(), pool, userID, req.Amount)
		if err != nil {
			if err == savings_sql.ErrInsufficientSavingsBalance {
				http.Error(w, "insufficient savings balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "could not withdraw", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
