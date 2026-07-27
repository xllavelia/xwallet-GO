package auth_http

import (
	"os"
	"time"

	"xwallet-server/users_sql"

	"github.com/golang-jwt/jwt/v5"
)

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-insecure-secret-change-me"
	}
	return []byte(secret)
}

func generateToken(user users_sql.User) (string, error) {
	claims := jwt.MapClaims{
		"playerId": user.PlayerID,
		"username": user.Username,
		"isAdmin":  user.IsAdmin,
		"exp":      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}
