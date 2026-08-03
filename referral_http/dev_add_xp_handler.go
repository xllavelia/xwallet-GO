package referral_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/referral_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type devAddXPRequest struct {
	Amount int `json:"amount"`
}

func DevAddXPHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req devAddXPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		if _, err := referral_sql.GetOrCreateByUserID(r.Context(), pool, userID); err != nil {
			http.Error(w, "could not prepare referral profile", http.StatusInternalServerError)
			return
		}

		if err := referral_sql.AddRefXP(r.Context(), pool, userID, req.Amount); err != nil {
			http.Error(w, "could not add xp", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
