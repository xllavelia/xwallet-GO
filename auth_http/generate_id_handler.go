package auth_http

import (
	"encoding/json"
	"math/rand"
	"net/http"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GenerateIDHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for attempt := 0; attempt < 20; attempt++ {
			candidate := randomDigits()

			exists, err := users_sql.PlayerIDExists(r.Context(), pool, candidate)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			if !exists {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"playerId": candidate})
				return
			}
		}
		http.Error(w, "could not find a free id, try again", http.StatusInternalServerError)
	}
}

func randomDigits() string {
	digits := "0123456789"
	out := make([]byte, 6)
	for i := range out {
		out[i] = digits[rand.Intn(len(digits))]
	}
	return string(out)
}
