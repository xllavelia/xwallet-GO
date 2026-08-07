package auth_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type updateUsernameRequest struct {
	Username string `json:"username"`
}

func UpdateUsernameHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authUser, ok := UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req updateUsernameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		newUsername := strings.TrimSpace(req.Username)
		if len(newUsername) < 3 {
			http.Error(w, "username must be at least 3 characters", http.StatusBadRequest)
			return
		}

		user, err := users_sql.GetUserByIdentifier(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		err = users_sql.UpdateUsername(r.Context(), pool, user.ID, newUsername)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "username already taken", http.StatusConflict)
				return
			}
			http.Error(w, "could not update username", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
