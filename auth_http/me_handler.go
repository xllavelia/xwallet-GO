package auth_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MeHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := users_sql.GetUserByIdentifier(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"playerId":  user.PlayerID,
			"username":  user.Username,
			"isAdmin":   user.IsAdmin,
			"createdAt": user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}
