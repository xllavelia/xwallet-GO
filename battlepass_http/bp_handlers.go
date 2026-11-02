package battlepass_http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/users_sql"
)

var errBPNoAuth = errors.New("unauthorized")

func bpWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func bpWriteError(w http.ResponseWriter, status int, message string) {
	bpWriteJSON(w, status, map[string]string{"error": message})
}

func bpUserID(r *http.Request, pool *pgxpool.Pool) (int, error) {
	u, ok := auth_http.UserFromContext(r)
	if !ok {
		return 0, errBPNoAuth
	}
	return users_sql.GetInternalIDByPlayerID(r.Context(), pool, u.PlayerID)
}

func bpError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBPNoAuth):
		bpWriteError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, battlepass_sql.BPErrPassRequired):
		bpWriteError(w, http.StatusForbidden, "battle pass required")
	case errors.Is(err, battlepass_sql.BPErrPassOwned):
		bpWriteError(w, http.StatusConflict, "battle pass already owned")
	case errors.Is(err, battlepass_sql.BPErrLevelLocked):
		bpWriteError(w, http.StatusConflict, "level is locked")
	case errors.Is(err, battlepass_sql.BPErrAlreadyClaimed):
		bpWriteError(w, http.StatusConflict, "reward already claimed")
	case errors.Is(err, battlepass_sql.BPErrInsufficientLavx):
		bpWriteError(w, http.StatusConflict, "not enough LAVX")
	case errors.Is(err, battlepass_sql.BPErrVoucherSlotsFull):
		bpWriteError(w, http.StatusConflict, "voucher slots are full — clear one and try again")
	case errors.Is(err, battlepass_sql.BPErrQuestNotFound):
		bpWriteError(w, http.StatusNotFound, "quest not found")
	case errors.Is(err, battlepass_sql.BPErrQuestNotComplete):
		bpWriteError(w, http.StatusConflict, "quest is not complete")
	case errors.Is(err, battlepass_sql.BPErrQuestClaimed):
		bpWriteError(w, http.StatusConflict, "quest already claimed")
	case errors.Is(err, battlepass_sql.BPErrQuestExpired):
		bpWriteError(w, http.StatusGone, "quest expired")
	case errors.Is(err, battlepass_sql.BPErrCaseNotFound):
		bpWriteError(w, http.StatusNotFound, "case not found")
	case errors.Is(err, battlepass_sql.BPErrWalletMissing):
		bpWriteError(w, http.StatusNotFound, "wallet not found")
	default:
		bpWriteError(w, http.StatusInternalServerError, "something went wrong")
	}
}

func BPStateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		state, err := battlepass_sql.BPLoadState(r.Context(), pool, userID)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, state)
	}
}

func BPPurchaseHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		res, err := battlepass_sql.BPPurchasePass(r.Context(), pool, userID)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, res)
	}
}

func BPClaimHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		var req struct {
			Level int    `json:"level"`
			Track string `json:"track"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			bpWriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		res, err := battlepass_sql.BPClaimReward(r.Context(), pool, userID, req.Level, req.Track)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, res)
	}
}

func BPQuestsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		res, err := battlepass_sql.BPLoadQuests(r.Context(), pool, userID)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, res)
	}
}

func BPQuestClaimHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		var req struct {
			QuestID int64 `json:"questId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			bpWriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		res, err := battlepass_sql.BPClaimQuest(r.Context(), pool, userID, req.QuestID)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, res)
	}
}

func BPCaseOpenHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			bpWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		userID, err := bpUserID(r, pool)
		if err != nil {
			bpError(w, err)
			return
		}
		var req struct {
			TokenID int64 `json:"tokenId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			bpWriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		res, err := battlepass_sql.BPOpenCase(r.Context(), pool, userID, req.TokenID)
		if err != nil {
			bpError(w, err)
			return
		}
		bpWriteJSON(w, http.StatusOK, res)
	}
}
