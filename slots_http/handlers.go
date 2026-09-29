package slots_http

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/slots_sql"
	"xwallet-server/users_sql"
)

// ============================================================
// АУТЕНТИФИКАЦИЯ: player_id из JWT -> внутренний int id пользователя
// (ровно так же, как в empire_http)
// ============================================================

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

// Единый формат ошибки по контракту фронта: {"error": msg}
func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": message})
}

// ============================================================
// GET /slots/state — полное свежее состояние
// ============================================================

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, err := slots_sql.BuildState(r.Context(), pool, userID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}

// ============================================================
// POST /slots/action — {"action":"spin","level_id":...} |
//                       {"action":"buy_pack","pack_id":...}
// Успех -> свежее полное состояние; ошибка -> 400 {"error": msg}
// ============================================================

func ActionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Action  string `json:"action"`
			LevelID string `json:"level_id"`
			PackID  string `json:"pack_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request", http.StatusBadRequest)
			return
		}
		var err error
		switch req.Action {
		case "spin":
			err = slots_sql.ActionSpin(r.Context(), pool, userID, req.LevelID)
		case "buy_pack":
			err = slots_sql.ActionBuyPack(r.Context(), pool, userID, req.PackID)
		default:
			writeError(w, "unknown action", http.StatusBadRequest)
			return
		}
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := slots_sql.BuildState(r.Context(), pool, userID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}
