package auth_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"xwallet-server/prime_sql"
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

		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "could not start registration", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		if err := users_sql.InsertUserTx(r.Context(), tx, req.PlayerID, req.Username, string(hash), false); err != nil {
			http.Error(w, "username or player id already taken", http.StatusConflict)
			return
		}
		user, err := users_sql.GetUserByPlayerIDTx(r.Context(), tx, req.PlayerID)
		if err != nil {
			http.Error(w, "could not load new user", http.StatusInternalServerError)
			return
		}
		if err := users_sql.UpdateUserDeviceTx(r.Context(), tx, req.PlayerID, strings.TrimSpace(r.Header.Get("X-Device-ID"))); err != nil {
			if errors.Is(err, users_sql.ErrDeviceBanned) {
				http.Error(w, "account or device banned", http.StatusForbidden)
				return
			}
			http.Error(w, "could not update device", http.StatusInternalServerError)
			return
		}
		if err := wallet_sql.InsertWalletTx(r.Context(), tx, user.ID); err != nil {
			http.Error(w, "could not create wallet", http.StatusInternalServerError)
			return
		}
		if err := referral_sql.CreateReferralForNewUserTx(r.Context(), tx, user.ID); err != nil {
			http.Error(w, "could not create referral profile", http.StatusInternalServerError)
			return
		}
		if err := user_vouchers_sql.GrantFeeDiscountVoucherTx(r.Context(), tx, user.ID, 100.00, 345600, "welcome", prime_sql.DefaultVoucherSlots); err != nil {
			http.Error(w, "could not create welcome voucher", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "could not complete registration", http.StatusInternalServerError)
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
