package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthState struct {
	Username     string
	IsAdmin      bool
	Banned       bool
	DeviceBanned bool
}

func GetAuthState(ctx context.Context, pool *pgxpool.Pool, playerID string) (AuthState, error) {
	var state AuthState
	err := pool.QueryRow(ctx, `
		SELECT u.username, u.is_admin, u.banned,
		       u.device_id IS NOT NULL AND u.device_id <> '' AND EXISTS (
		           SELECT 1 FROM banned_devices bd WHERE bd.device_id = u.device_id
		       )
		FROM users u
		WHERE u.player_id = $1;
	`, playerID).Scan(&state.Username, &state.IsAdmin, &state.Banned, &state.DeviceBanned)
	return state, err
}

const getUserByIdentifierQuery = `
	SELECT id, player_id, username, password_hash, is_admin, created_at
	FROM users
	WHERE player_id = $1 OR LOWER(username) = LOWER($1);
	`

func GetUserByIdentifier(ctx context.Context, pool *pgxpool.Pool, identifier string) (User, error) {
	return scanUser(pool.QueryRow(ctx, getUserByIdentifierQuery, identifier))
}

func GetUserByPlayerIDTx(ctx context.Context, tx pgx.Tx, playerID string) (User, error) {
	return scanUser(tx.QueryRow(ctx, `
		SELECT id, player_id, username, password_hash, is_admin, created_at
		FROM users WHERE player_id = $1;
	`, playerID))
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.PlayerID,
		&user.Username,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.CreatedAt,
	)
	return user, err
}
