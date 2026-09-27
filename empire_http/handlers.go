package empire_http

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/empire_sql"
	"xwallet-server/users_sql"
)

func writeOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth_http.UserFromContext(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		ctx := r.Context()
		userID, err := users_sql.GetInternalIDByPlayerID(ctx, pool, user.PlayerID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "User not found")
			return
		}
		state, err := empire_sql.BuildState(ctx, pool, userID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w, state)
	}
}

// ActionHandler — один эндпоинт на все действия.
// Тело: {"action": "...", ...payload}. После успеха возвращает
// свежее состояние (тот же формат, что и /empire/state).
func ActionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth_http.UserFromContext(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		ctx := r.Context()
		userID, err := users_sql.GetInternalIDByPlayerID(ctx, pool, user.PlayerID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "User not found")
			return
		}
		var req struct {
			Action        string `json:"action"`
			DefID         string `json:"def_id"`
			ObjectID      int    `json:"object_id"`
			Name          string `json:"name"`
			Direction     string `json:"direction"`
			AssetID       string `json:"asset_id"`
			Shares        int    `json:"shares"`
			ContractID    int    `json:"contract_id"`
			BoosterType   string `json:"booster_type"`
			DurationHours int    `json:"duration_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}

		switch req.Action {
		case "buy_object":
			err = empire_sql.ActionBuyObject(ctx, pool, userID, req.DefID)
		case "upgrade_object":
			err = empire_sql.ActionUpgradeObject(ctx, pool, userID, req.ObjectID)
		case "sell_object":
			err = empire_sql.ActionSellObject(ctx, pool, userID, req.ObjectID)
		case "rename_object":
			err = empire_sql.ActionRenameObject(ctx, pool, userID, req.ObjectID, req.Name)
		case "hire_worker":
			err = empire_sql.ActionHireWorker(ctx, pool, userID, req.ObjectID)
		case "buy_research":
			err = empire_sql.ActionBuyResearch(ctx, pool, userID, req.Direction)
		case "buy_asset":
			err = empire_sql.ActionBuyAsset(ctx, pool, userID, req.AssetID, req.Shares)
		case "sell_asset":
			err = empire_sql.ActionSellAsset(ctx, pool, userID, req.AssetID, req.Shares)
		case "claim_contract":
			err = empire_sql.ActionClaimContract(ctx, pool, userID, req.ContractID)
		case "buy_booster":
			err = empire_sql.ActionBuyBooster(ctx, pool, userID, req.BoosterType, req.DurationHours)
		case "prestige":
			err = empire_sql.ActionPrestige(ctx, pool, userID)
		default:
			writeErr(w, http.StatusBadRequest, "unknown action")
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}

		state, err := empire_sql.BuildState(ctx, pool, userID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w, state)
	}
}
