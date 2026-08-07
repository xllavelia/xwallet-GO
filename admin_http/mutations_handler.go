package admin_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/admin_sql"
	"xwallet-server/auth_http"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type playerAmountRequest struct {
	PlayerID string  `json:"playerId"`
	Balance  float64 `json:"balance"`
}

func SetBalanceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req playerAmountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Balance < 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := admin_sql.SetBalance(r.Context(), pool, userID, req.Balance); err != nil {
			http.Error(w, "could not set balance", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type playerLavxRequest struct {
	PlayerID string  `json:"playerId"`
	Lavx     float64 `json:"lavx"`
}

func SetLavxHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req playerLavxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Lavx < 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := admin_sql.SetLavx(r.Context(), pool, userID, req.Lavx); err != nil {
			http.Error(w, "could not set lavx", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type statusRequest struct {
	PlayerID string `json:"playerId"`
	Status   string `json:"status"`
}

func GrantStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req statusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := admin_sql.GrantStatus(r.Context(), pool, userID, req.Status); err != nil {
			http.Error(w, "could not grant status", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func RevokeStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req statusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := admin_sql.RevokeStatus(r.Context(), pool, userID, req.Status); err != nil {
			http.Error(w, "could not revoke status", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type setAdminRequest struct {
	PlayerID string `json:"playerId"`
	IsAdmin  bool   `json:"isAdmin"`
}

func SetAdminHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req setAdminRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		err := admin_sql.SetAdmin(r.Context(), pool, req.PlayerID, authUser.PlayerID, req.IsAdmin)
		if err != nil {
			if err == admin_sql.ErrCannotTargetSelf {
				http.Error(w, "cannot change your own admin status", http.StatusBadRequest)
				return
			}
			http.Error(w, "could not update admin status", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func DeleteUserHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		playerID := r.URL.Query().Get("playerId")
		if playerID == "" {
			http.Error(w, "playerId is required", http.StatusBadRequest)
			return
		}
		err := admin_sql.DeleteUser(r.Context(), pool, playerID, authUser.PlayerID)
		if err != nil {
			if err == admin_sql.ErrCannotTargetSelf {
				http.Error(w, "cannot delete your own account here — use Settings", http.StatusBadRequest)
				return
			}
			http.Error(w, "could not delete user", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
