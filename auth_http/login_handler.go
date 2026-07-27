package auth_http

import (
	"encoding/json"
	"net/http"
	"strings"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func LoginHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		identifier := strings.TrimSpace(req.Identifier)
		if identifier == "" || req.Password == "" {
			http.Error(w, "identifier and password are required", http.StatusBadRequest)
			return
		}

		user, err := users_sql.GetUserByIdentifier(r.Context(), pool, identifier)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := generateToken(user)
		if err != nil {
			http.Error(w, "could not create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(authResponse{
			Token:    token,
			PlayerID: user.PlayerID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		})
	}
}
