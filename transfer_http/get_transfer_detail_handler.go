package transfer_http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"xwallet-server/auth_http"
	"xwallet-server/transfer_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetTransferDetailHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid transfer id", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		detail, err := transfer_sql.GetTransferDetail(r.Context(), pool, id, userID)
		if err != nil {
			if err == transfer_sql.ErrForbidden {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if err == transfer_sql.ErrTransferNotFound {
				http.Error(w, "transfer not found", http.StatusNotFound)
				return
			}
			http.Error(w, "could not load transfer", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toDetailResponse(detail))
	}
}
