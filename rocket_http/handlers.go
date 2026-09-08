package rocket_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/rocket_engine"
	"xwallet-server/rocket_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type myBetDTO struct {
	Amount            float64  `json:"amount"`
	Status            string   `json:"status"`
	CashoutMultiplier *float64 `json:"cashoutMultiplier"`
	Payout            *float64 `json:"payout"`
}
type statsDTO struct {
	TotalBets     int     `json:"totalBets"`
	SuccessBets   int     `json:"successBets"`
	SuccessRate   float64 `json:"successRate"`
	MaxMultiplier float64 `json:"maxMultiplier"`
}
type userStatsDTO struct {
	TotalBets   int     `json:"totalBets"`
	WinBets     int     `json:"winBets"`
	WinRate     float64 `json:"winRate"`
	TotalProfit float64 `json:"totalProfit"`
	MaxWin      float64 `json:"maxWin"`
}
type stateResponse struct {
	Phase             string       `json:"phase"`
	RoundID           int          `json:"roundId"`
	SecondsRemaining  float64      `json:"secondsRemaining"`
	CurrentMultiplier float64      `json:"currentMultiplier"`
	MyBet             *myBetDTO    `json:"myBet"`
	RecentCrashes     []float64    `json:"recentCrashes"`
	GlobalStats       statsDTO     `json:"globalStats"`
	MyStats           userStatsDTO `json:"myStats"`
}

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		state := rocket_engine.GetPublicState()
		resp := stateResponse{
			Phase: state.Phase, RoundID: state.RoundID,
			SecondsRemaining: state.SecondsRemaining, CurrentMultiplier: state.CurrentMultiplier,
		}

		bet, err := rocket_sql.GetActiveBet(r.Context(), pool, userID, state.RoundID)
		if err == nil {
			resp.MyBet = &myBetDTO{Amount: bet.Amount, Status: bet.Status, CashoutMultiplier: bet.CashoutMultiplier, Payout: bet.Payout}
		}

		recent, _ := rocket_sql.GetRecentCrashPoints(r.Context(), pool, 12)
		resp.RecentCrashes = recent

		gs, _ := rocket_sql.GetGlobalStats(r.Context(), pool)
		successRate := 0.0
		if gs.TotalBets > 0 {
			successRate = (float64(gs.SuccessBets) / float64(gs.TotalBets)) * 100
		}
		resp.GlobalStats = statsDTO{TotalBets: gs.TotalBets, SuccessBets: gs.SuccessBets, SuccessRate: successRate, MaxMultiplier: gs.MaxMultiplier}

		us, _ := rocket_sql.GetUserStats(r.Context(), pool, userID)
		winRate := 0.0
		if us.TotalBets > 0 {
			winRate = (float64(us.WinBets) / float64(us.TotalBets)) * 100
		}
		resp.MyStats = userStatsDTO{TotalBets: us.TotalBets, WinBets: us.WinBets, WinRate: winRate, TotalProfit: us.TotalProfit, MaxWin: us.MaxWin}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

type betRequest struct {
	Amount float64 `json:"amount"`
}

func BetHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req betRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		_, err = rocket_engine.PlaceBet(r.Context(), userID, req.Amount)
		if err != nil {
			switch err {
			case rocket_sql.ErrNotBettingPhase:
				http.Error(w, "betting is closed, wait for the next round", http.StatusConflict)
			case rocket_sql.ErrInsufficientFunds:
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			case rocket_sql.ErrAlreadyBet:
				http.Error(w, "you already placed a bet for this round", http.StatusConflict)
			default:
				http.Error(w, "could not place bet", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type cashoutResponse struct {
	Multiplier float64 `json:"multiplier"`
	Payout     float64 `json:"payout"`
}

func CashoutHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		multiplier, payout, err := rocket_engine.CashOut(r.Context(), userID)
		if err != nil {
			switch err {
			case rocket_sql.ErrTooLate:
				http.Error(w, "too late, the round already crashed", http.StatusConflict)
			case rocket_sql.ErrNoActiveBet, rocket_sql.ErrRoundNotRunning:
				http.Error(w, "no active bet to cash out", http.StatusBadRequest)
			default:
				http.Error(w, "could not cash out", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cashoutResponse{Multiplier: multiplier, Payout: payout})
	}
}
