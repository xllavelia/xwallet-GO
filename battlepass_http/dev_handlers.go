package battlepass_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type devXpRequest struct {
	Amount int `json:"amount"`
}

func DevAddXpHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req devXpRequest
		json.NewDecoder(r.Body).Decode(&req)
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		battlepass_sql.DevAddXP(r.Context(), pool, userID, req.Amount)
		w.WriteHeader(http.StatusOK)
	}
}

func DevAddCaseHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		rarity := r.URL.Query().Get("rarity")
		if rarity != "epic" && rarity != "mythic" && rarity != "legendary" {
			http.Error(w, "invalid rarity", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		column := rarity + "_cases"
		sqlQuery := "UPDATE battlepass_progress SET " + column + " = " + column + " + 1 WHERE user_id = $1;"
		pool.Exec(r.Context(), sqlQuery, userID)
		w.WriteHeader(http.StatusOK)
	}
}
