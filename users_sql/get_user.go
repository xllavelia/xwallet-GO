package users_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetUserByIdentifier(ctx context.Context, pool *pgxpool.Pool, identifier string) (User, error) {
	sqlQuery := `
	SELECT id, player_id, username, password_hash, is_admin, created_at
	FROM users
	WHERE player_id = $1 OR LOWER(username) = LOWER($1);
	`
	var user User
	err := pool.QueryRow(ctx, sqlQuery, identifier).Scan(
		&user.ID,
		&user.PlayerID,
		&user.Username,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.CreatedAt,
	)

	return user, err
}
