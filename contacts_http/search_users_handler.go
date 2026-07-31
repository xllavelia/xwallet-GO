package contacts_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/contacts_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SearchUsersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		query := r.URL.Query().Get("q")
		if len(query) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]contacts_sql.UserSearchResult{})
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		results, err := contacts_sql.SearchUsers(r.Context(), pool, query, userID)
		if err != nil {
			http.Error(w, "search failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}
