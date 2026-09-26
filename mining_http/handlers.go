package mining_http

import (
	"encoding/json"
	"errors"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/mining_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

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

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// mapActionError переводит ошибки actions.go в HTTP-статус + текст.
func mapActionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wallet_sql.ErrInsufficientBalanceTx):
		writeJSONError(w, http.StatusBadRequest, "insufficient balance")
	case errors.Is(err, mining_sql.ErrUnknownServer), errors.Is(err, mining_sql.ErrUnknownItem), errors.Is(err, mining_sql.ErrUnknownDuration):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mining_sql.ErrSlotLimitReached):
		writeJSONError(w, http.StatusBadRequest, "server slot limit reached")
	case errors.Is(err, mining_sql.ErrServerNotFound):
		writeJSONError(w, http.StatusNotFound, "server not found")
	case errors.Is(err, mining_sql.ErrServerExhausted):
		writeJSONError(w, http.StatusBadRequest, "server is exhausted")
	case errors.Is(err, mining_sql.ErrAlreadyAlwaysOn):
		writeJSONError(w, http.StatusBadRequest, "server never sleeps")
	case errors.Is(err, mining_sql.ErrMaxRank):
		writeJSONError(w, http.StatusBadRequest, "server already at max rank")
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}

// ===================== GET /mining/state =====================

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		state, err := mining_sql.GetState(r.Context(), pool, userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}

// ===================== POST /mining/servers/buy =====================

func BuyServerHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body struct {
			CatalogID string `json:"catalogId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := mining_sql.BuyServer(r.Context(), pool, userID, body.CatalogID); err != nil {
			mapActionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ===================== POST /mining/servers/wake =====================

func WakeServerHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body struct {
			ServerID int64 `json:"serverId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := mining_sql.WakeServer(r.Context(), pool, userID, body.ServerID); err != nil {
			mapActionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ===================== POST /mining/servers/delete =====================

func DeleteServerHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body struct {
			ServerID int64 `json:"serverId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := mining_sql.DeleteServer(r.Context(), pool, userID, body.ServerID); err != nil {
			mapActionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ===================== POST /mining/servers/upgrade =====================

func UpgradeServerHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body struct {
			ServerID int64 `json:"serverId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := mining_sql.UpgradeServer(r.Context(), pool, userID, body.ServerID); err != nil {
			mapActionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ===================== POST /mining/items/buy =====================
// body.itemType: "no_sleep" | "profit_boost" | "power_boost" | "power_boost_eff" | "energy_pack" | "max_energy_expander" | "slot_expander"
// body.key: durationKey ("24h"/"3d"/"7d") для баффов, packId ("small"/"medium"/"large") для energy_pack, не нужен для остальных.

func BuyItemHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body struct {
			ItemType string `json:"itemType"`
			Key      string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}

		var err error
		switch body.ItemType {
		case mining_sql.BuffNoSleep, mining_sql.BuffProfitBoost, mining_sql.BuffPowerBoost, mining_sql.BuffPowerBoostEf:
			err = mining_sql.BuyBuffItem(r.Context(), pool, userID, body.ItemType, body.Key)
		case "energy_pack":
			err = mining_sql.BuyEnergyPack(r.Context(), pool, userID, body.Key)
		case "max_energy_expander":
			err = mining_sql.BuyMaxEnergyExpander(r.Context(), pool, userID)
		case "slot_expander":
			err = mining_sql.BuySlotExpander(r.Context(), pool, userID)
		default:
			writeJSONError(w, http.StatusBadRequest, "unknown item type")
			return
		}

		if err != nil {
			mapActionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
