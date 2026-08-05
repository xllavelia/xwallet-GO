package prime_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type purchaseRequest struct {
	Tier    string `json:"tier"`
	Billing string `json:"billing"`
}

func PurchaseHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req purchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		err = prime_sql.PurchaseSubscription(r.Context(), pool, userID, req.Tier, req.Billing)
		if err != nil {
			switch err {
			case prime_sql.ErrInvalidTier, prime_sql.ErrInvalidBilling:
				http.Error(w, err.Error(), http.StatusBadRequest)
			case prime_sql.ErrInsufficientLavx:
				http.Error(w, "insufficient LAVX balance", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not complete purchase", http.StatusInternalServerError)
			}
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
