package auth_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DeleteAccountHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.Header().Set("Allow", http.MethodDelete)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authUser, ok := UserFromContext(r)
		if !ok || authUser.PlayerID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := users_sql.DeleteUserByPlayerID(r.Context(), pool, authUser.PlayerID); err != nil {
			http.Error(w, "could not delete account", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "account deleted"})
	}
}
