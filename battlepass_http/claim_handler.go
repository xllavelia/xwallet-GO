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

type claimRequest struct {
	Level int `json:"level"`
}
type claimResponse struct {
	Components []battlepass_sql.RewardComponent `json:"components"`
}

func ClaimLevelHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req claimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		sub, err := prime_sql.GetActiveSubscription(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not check subscription", http.StatusInternalServerError)
			return
		}
		activeTier := ""
		maxSlots := prime_sql.DefaultVoucherSlots
		if sub != nil {
			activeTier = sub.Tier
			if cfg, ok := prime_sql.Tiers[sub.Tier]; ok {
				maxSlots = cfg.MaxVoucherSlots
			}
		}

		result, err := battlepass_sql.ClaimLevel(r.Context(), pool, userID, activeTier, req.Level, maxSlots)
		if err != nil {
			switch err {
			case battlepass_sql.ErrNoActiveTrack:
				http.Error(w, "no active battle pass — subscribe to Pro, Prime, or Star", http.StatusForbidden)
			case battlepass_sql.ErrLevelLocked:
				http.Error(w, "this level is still locked", http.StatusForbidden)
			case battlepass_sql.ErrAlreadyClaimed:
				http.Error(w, "already claimed", http.StatusConflict)
			case battlepass_sql.ErrLevelNotFound:
				http.Error(w, "level not found", http.StatusNotFound)
			default:
				http.Error(w, "could not claim reward", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claimResponse{Components: result.Components})
	}
}
