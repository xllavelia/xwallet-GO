package auth_http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"xwallet-server/users_sql"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const userContextKey contextKey = "authUser"

type AuthenticatedUser struct {
	PlayerID string
	Username string
	IsAdmin  bool
}

type AuthStateLookup func(context.Context, string) (users_sql.AuthState, error)

func RequireAuth(pool *pgxpool.Pool, next http.HandlerFunc) http.HandlerFunc {
	return RequireAuthWithLookup(func(ctx context.Context, playerID string) (users_sql.AuthState, error) {
		return users_sql.GetAuthState(ctx, pool, playerID)
	}, next)
}

func RequireAuthAllowingBanned(pool *pgxpool.Pool, next http.HandlerFunc) http.HandlerFunc {
	return requireAuthWithLookup(func(ctx context.Context, playerID string) (users_sql.AuthState, error) {
		return users_sql.GetAuthState(ctx, pool, playerID)
	}, true, next)
}

func RequireAuthWithLookup(lookup AuthStateLookup, next http.HandlerFunc) http.HandlerFunc {
	return requireAuthWithLookup(lookup, false, next)
}

func requireAuthWithLookup(lookup AuthStateLookup, allowBanned bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playerID, ok := parsePlayerIDFromRequest(r)
		if !ok {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		if lookup == nil {
			http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
			return
		}

		state, err := lookup(r.Context(), playerID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
			return
		}
		if !allowBanned && (state.Banned || state.DeviceBanned) {
			http.Error(w, "account or device banned", http.StatusForbidden)
			return
		}

		user := AuthenticatedUser{
			PlayerID: playerID,
			Username: state.Username,
			IsAdmin:  state.IsAdmin,
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func UserFromContext(r *http.Request) (AuthenticatedUser, bool) {
	user, ok := r.Context().Value(userContextKey).(AuthenticatedUser)
	return user, ok
}

func parsePlayerIDFromRequest(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if tokenString == "" {
		return "", false
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret(), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	playerID, _ := claims["playerId"].(string)
	return playerID, playerID != ""
}

func parseTokenFromRequest(r *http.Request, pool *pgxpool.Pool) (AuthenticatedUser, bool) {
	playerID, ok := parsePlayerIDFromRequest(r)
	if !ok {
		return AuthenticatedUser{}, false
	}
	state, err := users_sql.GetAuthState(r.Context(), pool, playerID)
	if err != nil || state.Banned || state.DeviceBanned {
		return AuthenticatedUser{}, false
	}
	return AuthenticatedUser{
		PlayerID: playerID,
		Username: state.Username,
		IsAdmin:  state.IsAdmin,
	}, true
}
