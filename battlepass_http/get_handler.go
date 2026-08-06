package battlepass_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type levelResponse struct {
	Level      int                              `json:"level"`
	Components []battlepass_sql.RewardComponent `json:"components"`
	Unlocked   bool                             `json:"unlocked"`
	Claimed    bool                             `json:"claimed"`
}

type statusResponse struct {
	HasActiveTrack bool            `json:"hasActiveTrack"`
	Track          string          `json:"track"`
	Xp             int             `json:"xp"`
	XpPerLevel     int             `json:"xpPerLevel"`
	Levels         []levelResponse `json:"levels"`
	EpicCases      int             `json:"epicCases"`
	MythicCases    int             `json:"mythicCases"`
	LegendaryCases int             `json:"legendaryCases"`
	Statuses       []string        `json:"statuses"`
}

func GetBattlePassHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		sub, err := prime_sql.GetActiveSubscription(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not check subscription", http.StatusInternalServerError)
			return
		}
		activeTier := ""
		if sub != nil {
			activeTier = sub.Tier
		}

		progress, err := battlepass_sql.GetProgress(r.Context(), pool, userID, activeTier)
		if err != nil {
			http.Error(w, "could not load battle pass", http.StatusInternalServerError)
			return
		}
		statuses, _ := battlepass_sql.GetStatuses(r.Context(), pool, userID)

		resp := statusResponse{
			HasActiveTrack: progress.Track != nil, Xp: progress.Xp, XpPerLevel: battlepass_sql.XPPerLevel,
			EpicCases: progress.EpicCases, MythicCases: progress.MythicCases, LegendaryCases: progress.LegendaryCases,
			Statuses: statuses, Levels: []levelResponse{},
		}

		if progress.Track != nil {
			resp.Track = *progress.Track
			catalog := battlepass_sql.CatalogForTrack(*progress.Track)
			claimedSet := map[int]bool{}
			for _, c := range progress.ClaimedLevels {
				claimedSet[c] = true
			}
			for _, lvl := range catalog {
				resp.Levels = append(resp.Levels, levelResponse{
					Level: lvl.Level, Components: lvl.Components,
					Unlocked: progress.Xp >= lvl.Level*battlepass_sql.XPPerLevel, Claimed: claimedSet[lvl.Level],
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
