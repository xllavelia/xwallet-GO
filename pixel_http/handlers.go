package pixel_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/pixel_engine"
	"xwallet-server/pixel_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

		state := pixel_engine.GetPublicState()
		boardState, _ := pixel_engine.GetMyBoard(r.Context(), userID)

		gs, _ := pixel_sql.GetGlobalStats(r.Context(), pool)
		successRate := 0.0
		if gs.TotalGames > 0 {
			successRate = (float64(gs.SuccessGames) / float64(gs.TotalGames)) * 100
		}
		us, _ := pixel_sql.GetUserStats(r.Context(), pool, userID)
		winRate := 0.0
		if us.TotalGames > 0 {
			winRate = (float64(us.GamesWon) / float64(us.TotalGames)) * 100
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"phase": state.Phase, "roundId": state.RoundID, "secondsRemaining": state.SecondsRemaining,
			"config": map[string]interface{}{
				"gridCols": pixel_sql.GridCols, "gridRows": pixel_sql.GridRows,
				"mineCount": pixel_sql.MineCount(), "percentPerCell": pixel_sql.PercentPerCell,
				"livesCount": pixel_sql.LivesCount,
			},
			"myBoard": map[string]interface{}{
				"exists": boardState.Exists, "status": boardState.Status, "livesRemaining": boardState.LivesRemaining,
				"revealedCells": boardState.RevealedCells, "safeCellsCount": boardState.SafeCellsCount,
				"currentMultiplier": boardState.CurrentMultiplier, "currentPayout": boardState.CurrentPayout,
				"amount": boardState.Amount, "minePositions": boardState.MinePositions, "payout": boardState.Payout,
			},
			"globalStats": map[string]interface{}{"totalGames": gs.TotalGames, "successGames": gs.SuccessGames, "successRate": successRate, "maxPayout": gs.MaxPayout},
			"myStats": map[string]interface{}{
				"totalCellsOpened": us.TotalCellsOpened, "safeCellsOpened": us.SafeCellsOpened,
				"totalGames": us.TotalGames, "gamesWon": us.GamesWon, "winRate": winRate,
				"totalProfit": us.TotalProfit, "maxWin": us.MaxWin,
			},
		})
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
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req betRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
		_, err := pixel_engine.PlaceBet(r.Context(), userID, req.Amount)
		if err != nil {
			switch err {
			case pixel_sql.ErrNotBettingPhase:
				http.Error(w, "betting is closed, wait for the next round", http.StatusConflict)
			case pixel_sql.ErrInsufficientFunds:
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
			case pixel_sql.ErrAlreadyBet:
				http.Error(w, "you already placed a bet for this round", http.StatusConflict)
			default:
				http.Error(w, "could not place bet", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type revealRequest struct {
	CellIndex int `json:"cellIndex"`
}

func RevealHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req revealRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result, err := pixel_engine.RevealCell(r.Context(), userID, req.CellIndex)
		if err != nil {
			switch err {
			case pixel_sql.ErrNotRunningPhase:
				http.Error(w, "the game is not currently running", http.StatusConflict)
			case pixel_sql.ErrNoActiveBoard:
				http.Error(w, "no active board — place a bet first", http.StatusBadRequest)
			case pixel_sql.ErrCellAlreadyRevealed:
				http.Error(w, "this cell is already revealed", http.StatusConflict)
			case pixel_sql.ErrInvalidCell:
				http.Error(w, "invalid cell", http.StatusBadRequest)
			default:
				http.Error(w, "could not reveal cell", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isMine": result.IsMine, "livesRemaining": result.LivesRemaining, "safeCellsCount": result.SafeCellsCount,
			"currentMultiplier": result.CurrentMultiplier, "currentPayout": result.CurrentPayout,
			"gameOver": result.GameOver, "minePositions": result.MinePositions,
		})
	}
}
func CashoutHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		payout, minePositions, err := pixel_engine.CashOut(r.Context(), userID)
		if err != nil {
			switch err {
			case pixel_sql.ErrNotRunningPhase:
				http.Error(w, "the game is not currently running", http.StatusConflict)
			case pixel_sql.ErrNoActiveBoard:
				http.Error(w, "no active board to cash out", http.StatusBadRequest)
			default:
				http.Error(w, "could not cash out", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"payout":        payout,
			"minePositions": minePositions,
		})
	}
}
