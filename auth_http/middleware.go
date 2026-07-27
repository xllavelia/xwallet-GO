package auth_http

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userContextKey contextKey = "authUser"

type AuthenticatedUser struct {
	PlayerID string
	Username string
	IsAdmin  bool
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "missing or invalid authorization header", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret(), nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}

		user := AuthenticatedUser{
			PlayerID: toString(claims["playerId"]),
			Username: toString(claims["username"]),
			IsAdmin:  toBool(claims["isAdmin"]),
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func UserFromContext(r *http.Request) (AuthenticatedUser, bool) {
	user, ok := r.Context().Value(userContextKey).(AuthenticatedUser)
	return user, ok
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func toBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}
