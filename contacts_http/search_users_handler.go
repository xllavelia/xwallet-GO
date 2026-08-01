package contacts_http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		query := r.URL.Query().Get("q")

		w.Header().Set("Content-Type", "application/json")

		if len(query) == 0 {
			json.NewEncoder(w).Encode([]contacts_sql.UserSearchResult{})
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(ctx, pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "could not resolve your account", http.StatusInternalServerError)
			return
		}

		results, err := contacts_sql.SearchUsers(ctx, pool, query, userID)
		if err != nil {
			http.Error(w, "search query failed", http.StatusInternalServerError)
			return
		}

		if results == nil {
			results = []contacts_sql.UserSearchResult{}
		}

		json.NewEncoder(w).Encode(results)
	}
}
