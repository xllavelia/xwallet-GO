package user_vouchers_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type activateRequest struct {
	ID int `json:"id"`
}

type activateResponse struct {
	VoucherType  string  `json:"voucherType"`
	CreditAmount float64 `json:"creditAmount"`
}

func ActivateVoucherHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req activateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		affected, err := user_vouchers_sql.ActivateFeeVoucher(r.Context(), pool, req.ID, userID)
		if err != nil {
			http.Error(w, "could not activate voucher", http.StatusInternalServerError)
			return
		}
		if affected {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(activateResponse{VoucherType: "fee_discount"})
			return
		}

		voucherType, amount, err := user_vouchers_sql.ClaimCreditVoucher(r.Context(), pool, req.ID, userID)
		if err != nil {
			if err == user_vouchers_sql.ErrVoucherNotFound {
				http.Error(w, "voucher not found or already used", http.StatusNotFound)
				return
			}
			http.Error(w, "could not claim voucher", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(activateResponse{VoucherType: voucherType, CreditAmount: amount})
	}
}
