package contacts_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/contacts_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type removeContactRequest struct {
	ContactPlayerID string `json:"contactPlayerId"`
}

func RemoveContactHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req removeContactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		contactUserID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, req.ContactPlayerID)
		if err != nil {
			http.Error(w, "contact not found", http.StatusNotFound)
			return
		}

		if err := contacts_sql.RemoveContact(r.Context(), pool, userID, contactUserID); err != nil {
			http.Error(w, "could not remove contact", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
