package admin_http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"xwallet-server/admin_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type userRowResponse struct {
	PlayerID    string  `json:"playerId"`
	Username    string  `json:"username"`
	IsAdmin     bool    `json:"isAdmin"`
	CreatedAt   string  `json:"createdAt"`
	Balance     float64 `json:"balance"`
	LavxBalance float64 `json:"lavxBalance"`
	PrimeTier   string  `json:"primeTier"`
}

func ListUsersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("q")
		limit := 50
		offset := 0
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
			limit = l
		}
		if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
			offset = o
		}

		rows, err := admin_sql.ListUsers(r.Context(), pool, search, limit, offset)
		if err != nil {
			http.Error(w, "could not load users", http.StatusInternalServerError)
			return
		}

		items := make([]userRowResponse, 0, len(rows))
		for _, u := range rows {
			tier := ""
			if u.PrimeTier != nil {
				tier = *u.PrimeTier
			}
			items = append(items, userRowResponse{
				PlayerID: u.PlayerID, Username: u.Username, IsAdmin: u.IsAdmin,
				CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
				Balance:   u.Balance, LavxBalance: u.LavxBalance, PrimeTier: tier,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

type userDetailResponse struct {
	PlayerID        string   `json:"playerId"`
	Username        string   `json:"username"`
	IsAdmin         bool     `json:"isAdmin"`
	CreatedAt       string   `json:"createdAt"`
	Balance         float64  `json:"balance"`
	LavxBalance     float64  `json:"lavxBalance"`
	PrimeTier       string   `json:"primeTier"`
	ReferralCode    string   `json:"referralCode"`
	RefXp           int      `json:"refXp"`
	BattlepassTrack string   `json:"battlepassTrack"`
	BattlepassXp    int      `json:"battlepassXp"`
	OpenPositions   int      `json:"openPositions"`
	ClosedPositions int      `json:"closedPositions"`
	VoucherCount    int      `json:"voucherCount"`
	Statuses        []string `json:"statuses"`
}

func GetUserDetailHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playerID := r.URL.Query().Get("playerId")
		if playerID == "" {
			http.Error(w, "playerId is required", http.StatusBadRequest)
			return
		}

		d, err := admin_sql.GetUserDetail(r.Context(), pool, playerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		statuses := d.Statuses
		if statuses == nil {
			statuses = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userDetailResponse{
			PlayerID: d.PlayerID, Username: d.Username, IsAdmin: d.IsAdmin,
			CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Balance:   d.Balance, LavxBalance: d.LavxBalance, PrimeTier: d.PrimeTier,
			ReferralCode: d.ReferralCode, RefXp: d.RefXp,
			BattlepassTrack: d.BattlepassTrack, BattlepassXp: d.BattlepassXp,
			OpenPositions: d.OpenPositions, ClosedPositions: d.ClosedPositions,
			VoucherCount: d.VoucherCount, Statuses: statuses,
		})
	}
}
