package admin_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/admin_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type statsResponse struct {
	TotalUsers      int     `json:"totalUsers"`
	TotalBalance    float64 `json:"totalBalance"`
	TotalLavx       float64 `json:"totalLavx"`
	OpenPositions   int     `json:"openPositions"`
	ClosedPositions int     `json:"closedPositions"`
	TotalVolume     float64 `json:"totalVolume"`
	TotalTransfers  int     `json:"totalTransfers"`
	TransferVolume  float64 `json:"transferVolume"`
	TotalVouchers   int     `json:"totalVouchers"`
	ActiveSubs      int     `json:"activeSubs"`
}

func GetStatsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := admin_sql.GetPlatformStats(r.Context(), pool)
		if err != nil {
			http.Error(w, "could not load stats", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResponse{
			TotalUsers: s.TotalUsers, TotalBalance: s.TotalBalance, TotalLavx: s.TotalLavx,
			OpenPositions: s.OpenPositions, ClosedPositions: s.ClosedPositions, TotalVolume: s.TotalVolume,
			TotalTransfers: s.TotalTransfers, TransferVolume: s.TransferVolume,
			TotalVouchers: s.TotalVouchers, ActiveSubs: s.ActiveSubs,
		})
	}
}
