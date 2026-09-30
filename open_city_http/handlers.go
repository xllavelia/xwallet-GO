package open_city_http

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/open_city_sql"
	"xwallet-server/users_sql"
)

// ============================================================
// АУТЕНТИФИКАЦИЯ — общая JWT/auth система XWallet (как в slots_http)
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

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": message})
}

// ============================================================
// GET /opencity/state — полное состояние; при первом входе
// состояние создаётся само и отдаётся уже готовое.
// ============================================================

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, err := open_city_sql.BuildState(r.Context(), pool, userID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}

// ============================================================
// POST /opencity/action
//   {"action":"move","location":"downtown","x":12,"y":-5}
//   {"action":"claim_reward","reward_id":"first_visit"}
//   {"action":"inventory","op":"add","item_id":"apple_pie","qty":2}
//   {"action":"save_progress","patch":{"tutorial_step":3}}
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
			Action   string                 `json:"action"`
			Location string                 `json:"location"`
			X        float64                `json:"x"`
			Y        float64                `json:"y"`
			RewardID string                 `json:"reward_id"`
			Op       string                 `json:"op"`
			ItemID   string                 `json:"item_id"`
			Qty      int                    `json:"qty"`
			Patch    map[string]interface{} `json:"patch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "invalid request", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		var err error
		switch req.Action {
		case "move":
			err = open_city_sql.ActionMove(ctx, pool, userID, req.Location, req.X, req.Y)
		case "claim_reward":
			err = open_city_sql.ActionClaimReward(ctx, pool, userID, req.RewardID)
		case "inventory":
			err = open_city_sql.ActionInventory(ctx, pool, userID, req.Op, req.ItemID, req.Qty)
		case "save_progress":
			err = open_city_sql.ActionSaveProgress(ctx, pool, userID, req.Patch)
		default:
			writeError(w, "unknown action", http.StatusBadRequest)
			return
		}
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		state, err := open_city_sql.BuildState(ctx, pool, userID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}
