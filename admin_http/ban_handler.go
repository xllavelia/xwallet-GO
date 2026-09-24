package admin_http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type banRequest struct {
	PlayerID string `json:"playerId"`
	Device   bool   `json:"device"`
}

func BanUserHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req banRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var err error
		if req.Device {
			err = users_sql.BanDevice(r.Context(), pool, req.PlayerID)
		} else {
			err = users_sql.BanAccount(r.Context(), pool, req.PlayerID)
		}
		if err != nil {
			switch {
			case errors.Is(err, users_sql.ErrNoDeviceOnRecord):
				http.Error(w, "this user has no device id on record yet", http.StatusConflict)
			case errors.Is(err, pgx.ErrNoRows):
				http.Error(w, "user not found", http.StatusNotFound)
			default:
				http.Error(w, "could not ban user", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type unbanRequest struct {
	PlayerID string `json:"playerId"`
}

func UnbanUserHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req unbanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := users_sql.UnbanUser(r.Context(), pool, req.PlayerID); err != nil {
			http.Error(w, "could not unban user", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type maintenanceRequest struct {
	Minutes *int `json:"minutes"`
}

func SetMaintenanceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req maintenanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var until *time.Time
		if req.Minutes != nil {
			if *req.Minutes <= 0 {
				http.Error(w, "minutes must be positive", http.StatusBadRequest)
				return
			}
			t := time.Now().Add(time.Duration(*req.Minutes) * time.Minute)
			until = &t
		}
		if err := users_sql.SetMaintenance(r.Context(), pool, until); err != nil {
			http.Error(w, "could not set maintenance", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func ClearMaintenanceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := users_sql.ClearMaintenance(r.Context(), pool); err != nil {
			http.Error(w, "could not clear maintenance", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type maintenanceResponse struct {
	Maintenance bool   `json:"maintenance"`
	Until       string `json:"until"`
}

func GetMaintenanceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		active, until, err := users_sql.GetMaintenance(r.Context(), pool)
		if err != nil {
			http.Error(w, "could not load maintenance", http.StatusInternalServerError)
			return
		}
		untilStr := ""
		if until != nil {
			untilStr = until.UTC().Format(time.RFC3339)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(maintenanceResponse{Maintenance: active, Until: untilStr})
	}
}
