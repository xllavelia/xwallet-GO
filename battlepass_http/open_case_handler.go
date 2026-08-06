package battlepass_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type openCaseRequest struct {
	Rarity string `json:"rarity"`
}

func OpenCaseHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req openCaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		maxSlots := 5
		if sub, err := prime_sql.GetActiveSubscription(r.Context(), pool, userID); err == nil && sub != nil {
			if cfg, ok := prime_sql.Tiers[sub.Tier]; ok {
				maxSlots = cfg.MaxVoucherSlots
			}
		}

		result, err := battlepass_sql.OpenCase(r.Context(), pool, userID, req.Rarity, maxSlots)
		if err != nil {
			if err == battlepass_sql.ErrNoCasesAvailable {
				http.Error(w, "no cases of this rarity available", http.StatusConflict)
				return
			}
			http.Error(w, "could not open case", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
