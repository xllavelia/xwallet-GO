package auth_http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"xwallet-server/referral_sql"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var playerIDPattern = regexp.MustCompile(`^[0-9]{6}$`)

type registerRequest struct {
	Username string `json:"username"`
	PlayerID string `json:"playerId"`
	Password string `json:"password"`
}

type authResponse struct {
	Token    string `json:"token"`
	PlayerID string `json:"playerId"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}

func RegisterHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		req.PlayerID = strings.TrimSpace(req.PlayerID)

		if len(req.Username) < 3 {
			http.Error(w, "username must be at least 3 characters", http.StatusBadRequest)
			return
		}
		if !playerIDPattern.MatchString(req.PlayerID) {
			http.Error(w, "player id must be exactly 6 digits", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 6 {
			http.Error(w, "password must be at least 6 characters", http.StatusBadRequest)
			return
		}

		exists, err := users_sql.PlayerIDExists(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if exists {
			http.Error(w, "player id already taken", http.StatusConflict)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "could not process password", http.StatusInternalServerError)
			return
		}

		if err := users_sql.InsertUser(r.Context(), pool, req.PlayerID, req.Username, string(hash), false); err != nil {
			http.Error(w, "username or player id already taken", http.StatusConflict)
			return
		}

		user, err := users_sql.GetUserByIdentifier(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user was created but could not be loaded", http.StatusInternalServerError)
			return
		}

		if err := wallet_sql.InsertWallet(r.Context(), pool, user.ID); err != nil {
			http.Error(w, "could not create wallet", http.StatusInternalServerError)
			return
		}
		if err := referral_sql.CreateReferralForNewUser(r.Context(), pool, user.ID); err != nil {
			http.Error(w, "could not create referral profile", http.StatusInternalServerError)
			return
		}
		if err := user_vouchers_sql.GrantFeeDiscountVoucher(r.Context(), pool, user.ID, 100.00, 345600, "welcome", 5); err != nil {
			http.Error(w, "could not create welcome voucher", http.StatusInternalServerError)
			return
		}
		token, err := generateToken(user)
		if err != nil {
			http.Error(w, "could not create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(authResponse{
			Token:    token,
			PlayerID: user.PlayerID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		})
	}
}
