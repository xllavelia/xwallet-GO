package user_vouchers_http

import (
	"net/http"
	"strconv"

	"xwallet-server/auth_http"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DeleteVoucherHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "invalid voucher id", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		affected, err := user_vouchers_sql.DeleteFeeVoucher(r.Context(), pool, id, userID)
		if err != nil {
			http.Error(w, "could not delete voucher", http.StatusInternalServerError)
			return
		}
		if !affected {
			http.Error(w, "voucher is currently active and cannot be deleted", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
