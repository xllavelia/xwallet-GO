package user_vouchers_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type devGrantRequest struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
}

// Placeholder until the promo-code redemption flow is built — lets you
// test credit/LAVX voucher rendering & claiming without real promo codes yet.
func DevGrantHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req devGrantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		var grantErr error
		if req.Type == "fee_discount" {
			grantErr = user_vouchers_sql.GrantFeeDiscountVoucher(r.Context(), pool, userID, req.Amount, 345600, "dev")
		} else if req.Type == "usdt_credit" || req.Type == "lavx_credit" || req.Type == "ref_xp_credit" {
			grantErr = user_vouchers_sql.GrantCreditVoucher(r.Context(), pool, userID, req.Type, req.Amount, "dev")
		} else {
			http.Error(w, "invalid voucher type", http.StatusBadRequest)
			return
		}

		if grantErr != nil {
			if grantErr == user_vouchers_sql.ErrSlotsFull {
				http.Error(w, "all 5 voucher slots are full", http.StatusConflict)
				return
			}
			http.Error(w, "could not grant voucher", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func DevResetHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		if err := user_vouchers_sql.DeleteAllByUserID(r.Context(), pool, userID); err != nil {
			http.Error(w, "could not reset vouchers", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
