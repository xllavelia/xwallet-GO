package contacts_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/contacts_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type searchResponse struct {
	DebugBuild string                          `json:"debugBuild"`
	Query      string                          `json:"query"`
	Count      int                             `json:"count"`
	Results    []contacts_sql.UserSearchResult `json:"results"`
}

func SearchUsersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		query := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")

		if len(query) == 0 {
			json.NewEncoder(w).Encode(searchResponse{
				DebugBuild: "search-rewrite-v2",
				Query:      query,
				Count:      0,
				Results:    []contacts_sql.UserSearchResult{},
			})
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "could not resolve your account", http.StatusInternalServerError)
			return
		}

		results, err := contacts_sql.SearchUsers(r.Context(), pool, query, userID)
		if err != nil {
			http.Error(w, "search query failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(searchResponse{
			DebugBuild: "search-rewrite-v2",
			Query:      query,
			Count:      len(results),
			Results:    results,
		})
	}
}
