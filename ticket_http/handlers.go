package ticket_http

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/ticket_sql"
	"xwallet-server/users_sql"
)

func getUserID(r *http.Request, pool *pgxpool.Pool) (int, bool) {
	authUser, ok := auth_http.UserFromContext(r)
	if !ok {
		return 0, false
	}
	userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, err := ticket_sql.GetState(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load state", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}

type rarityRequest struct {
	Rarity string `json:"rarity"`
}

func BuyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req rarityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rarity == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := ticket_sql.BuyPack(r.Context(), pool, userID, req.Rarity); err != nil {
			switch err {
			case ticket_sql.ErrUnknownRarity:
				http.Error(w, "unknown pack", http.StatusBadRequest)
			case ticket_sql.ErrInsufficientFunds:
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not buy pack", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func OpenHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req rarityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rarity == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result, err := ticket_sql.OpenTicket(r.Context(), pool, userID, req.Rarity)
		if err != nil {
			switch err {
			case ticket_sql.ErrUnknownRarity:
				http.Error(w, "unknown pack", http.StatusBadRequest)
			case ticket_sql.ErrNoTickets:
				http.Error(w, "no tickets of this rarity left", http.StatusConflict)
			default:
				http.Error(w, "could not open ticket", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
