package auth_http

import (
	"encoding/json"
	"net/http"
)

func MeHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"playerId": user.PlayerID,
		"username": user.Username,
		"isAdmin":  user.IsAdmin,
	})
}
