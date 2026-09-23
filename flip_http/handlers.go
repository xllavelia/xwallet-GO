package flip_http

import (
	"encoding/json"
	"errors"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/flip_engine"
	"xwallet-server/flip_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type betDTO struct {
	Amount float64  `json:"amount"`
	Choice string   `json:"choice"`
	Status string   `json:"status"`
	Payout *float64 `json:"payout"`
}

type statsDTO struct {
	TotalBets int     `json:"totalBets"`
	WonBets   int     `json:"wonBets"`
	WinRate   float64 `json:"winRate"`
	MaxPayout float64 `json:"maxPayout"`
}

type userStatsDTO struct {
	TotalBets   int     `json:"totalBets"`
	WonBets     int     `json:"wonBets"`
	WinRate     float64 `json:"winRate"`
	TotalProfit float64 `json:"totalProfit"`
	MaxWin      float64 `json:"maxWin"`
}

type stateResponse struct {
	Phase            string       `json:"phase"`
	RoundID          int          `json:"roundId"`
	SecondsRemaining float64      `json:"secondsRemaining"`
	FixedMultiplier  float64      `json:"fixedMultiplier"`
	Outcome          string       `json:"outcome"`
	LastOutcome      string       `json:"lastOutcome"`
	MyBet            *betDTO      `json:"myBet"`
	RecentOutcomes   []string     `json:"recentOutcomes"`
	GlobalStats      statsDTO     `json:"globalStats"`
	MyStats          userStatsDTO `json:"myStats"`
}

func getUserID(r *http.Request, pool *pgxpool.Pool) (int, bool) {
	authUser, ok := auth_http.UserFromContext(r)
	if !ok {
		return 0, false
	}

	userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		state := flip_engine.GetPublicState()
		response := stateResponse{
			Phase:            state.Phase,
			RoundID:          state.RoundID,
			SecondsRemaining: state.SecondsRemaining,
			FixedMultiplier:  flip_sql.PayoutMultiplier,
			Outcome:          state.Outcome,
			LastOutcome:      state.LastOutcome,
			RecentOutcomes:   []string{},
		}

		bet, err := flip_sql.GetBetForRound(r.Context(), pool, userID, state.RoundID)
		if err == nil {
			response.MyBet = &betDTO{
				Amount: bet.Amount,
				Choice: bet.Choice,
				Status: bet.Status,
				Payout: bet.Payout,
			}
		}

		recent, err := flip_sql.GetRecentOutcomes(r.Context(), pool, 10)
		if err == nil {
			response.RecentOutcomes = recent
		}

		global, err := flip_sql.GetGlobalStats(r.Context(), pool)
		if err == nil {
			winRate := 0.0
			if global.TotalBets > 0 {
				winRate = float64(global.WonBets) / float64(global.TotalBets) * 100
			}
			response.GlobalStats = statsDTO{
				TotalBets: global.TotalBets,
				WonBets:   global.WonBets,
				WinRate:   winRate,
				MaxPayout: global.MaxPayout,
			}
		}

		userStats, err := flip_sql.GetUserStats(r.Context(), pool, userID)
		if err == nil {
			winRate := 0.0
			if userStats.TotalBets > 0 {
				winRate = float64(userStats.WonBets) / float64(userStats.TotalBets) * 100
			}
			response.MyStats = userStatsDTO{
				TotalBets:   userStats.TotalBets,
				WonBets:     userStats.WonBets,
				WinRate:     winRate,
				TotalProfit: userStats.TotalProfit,
				MaxWin:      userStats.MaxWin,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

type betRequest struct {
	Amount float64 `json:"amount"`
	Choice string  `json:"choice"`
}

func BetHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var request betRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		bet, err := flip_engine.PlaceBet(r.Context(), userID, request.Amount, request.Choice)
		if err != nil {
			switch {
			case errors.Is(err, flip_sql.ErrInvalidChoice), errors.Is(err, flip_sql.ErrInvalidAmount):
				http.Error(w, "invalid bet", http.StatusBadRequest)
			case errors.Is(err, flip_sql.ErrNotBettingPhase):
				http.Error(w, "betting is closed, wait for the next round", http.StatusConflict)
			case errors.Is(err, flip_sql.ErrAlreadyBet):
				http.Error(w, "you already placed a bet for this round", http.StatusConflict)
			case errors.Is(err, flip_sql.ErrInsufficientFunds):
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			case errors.Is(err, pgx.ErrNoRows):
				http.Error(w, "round not found", http.StatusConflict)
			default:
				http.Error(w, "could not place bet", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"betId":   bet.ID,
			"roundId": bet.RoundID,
			"amount":  bet.Amount,
			"choice":  bet.Choice,
		})
	}
}
