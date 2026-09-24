package auth_http

import (
	"encoding/json"
	"net/http"
	"time"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type authStatusResponse struct {
	AccountBanned bool   `json:"accountBanned"`
	BanReason     string `json:"banReason"`
	DeviceBanned  bool   `json:"deviceBanned"`
}

func AuthStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		st, err := users_sql.GetBanStatus(r.Context(), pool, user.PlayerID)
		if err != nil {
			http.Error(w, "could not load status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authStatusResponse{
			AccountBanned: st.AccountBanned,
			BanReason:     st.BanReason,
			DeviceBanned:  st.DeviceBanned,
		})
	}
}

type appStatusResponse struct {
	Maintenance bool   `json:"maintenance"`
	Until       string `json:"until"`
}

func AppStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		active, until, err := users_sql.GetMaintenance(r.Context(), pool)
		if err != nil {
			http.Error(w, "could not load status", http.StatusInternalServerError)
			return
		}
		untilStr := ""
		if until != nil {
			untilStr = until.UTC().Format(time.RFC3339)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appStatusResponse{
			Maintenance: active,
			Until:       untilStr,
		})
	}
}
