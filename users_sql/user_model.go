package users_sql

import "time"

type User struct {
	ID           int
	PlayerID     string
	Username     string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
}
