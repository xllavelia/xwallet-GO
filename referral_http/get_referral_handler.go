package referral_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/referral_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type referralResponse struct {
	ReferralCode        string  `json:"referralCode"`
	Level               int     `json:"level"`
	LevelName           string  `json:"levelName"`
	CommissionPercent   float64 `json:"commissionPercent"`
	RefXp               int     `json:"refXp"`
	RefXpIntoLevel      int     `json:"refXpIntoLevel"`
	RefXpForLevel       int     `json:"refXpForLevel"`
	IsMaxLevel          bool    `json:"isMaxLevel"`
	NextLevelName       string  `json:"nextLevelName"`
	NextLevelRate       float64 `json:"nextLevelRate"`
	FriendsInvited      int     `json:"friendsInvited"`
	ActiveTraders       int     `json:"activeTraders"`
	ConversionRate      float64 `json:"conversionRate"`
	TotalReferredVolume float64 `json:"totalReferredVolume"`
	TotalEarned         float64 `json:"totalEarned"`
}

func GetReferralHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		ref, err := referral_sql.GetOrCreateByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load referral profile", http.StatusInternalServerError)
			return
		}

		friendsInvited, err := referral_sql.CountFriendsInvited(r.Context(), pool, userID)
		if err != nil {
			friendsInvited = 0
		}
		activeTraders, err := referral_sql.CountActiveTraders(r.Context(), pool, userID)
		if err != nil {
			activeTraders = 0
		}

		level := referral_sql.LevelFromXP(ref.RefXp)
		isMax := level >= 5

		resp := referralResponse{
			ReferralCode:        ref.ReferralCode,
			Level:               level,
			LevelName:           referral_sql.NameForLevel(level),
			CommissionPercent:   referral_sql.RateForLevel(level),
			RefXp:               ref.RefXp,
			IsMaxLevel:          isMax,
			FriendsInvited:      friendsInvited,
			ActiveTraders:       activeTraders,
			TotalReferredVolume: ref.TotalReferredVolume,
			TotalEarned:         ref.TotalEarned,
		}

		if friendsInvited > 0 {
			resp.ConversionRate = (float64(activeTraders) / float64(friendsInvited)) * 100
		}

		if !isMax {
			resp.RefXpIntoLevel = ref.RefXp - (level-1)*100
			resp.RefXpForLevel = 100
			resp.NextLevelName = referral_sql.NameForLevel(level + 1)
			resp.NextLevelRate = referral_sql.RateForLevel(level + 1)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
