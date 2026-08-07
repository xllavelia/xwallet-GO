package admin_sql

import (
	"context"
	"errors"

	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCannotTargetSelf = errors.New("cannot perform this action on your own account")

func SetBalance(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) error {
	_, err := pool.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = now() WHERE user_id = $2;`, amount, userID)
	return err
}

func SetLavx(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) error {
	_, err := pool.Exec(ctx, `UPDATE wallets SET lavx_balance = $1 WHERE user_id = $2;`, amount, userID)
	return err
}

func GrantStatus(ctx context.Context, pool *pgxpool.Pool, userID int, status string) error {
	_, err := pool.Exec(ctx, `INSERT INTO user_statuses (user_id, status) VALUES ($1, $2) ON CONFLICT (user_id, status) DO NOTHING;`, userID, status)
	return err
}

func RevokeStatus(ctx context.Context, pool *pgxpool.Pool, userID int, status string) error {
	_, err := pool.Exec(ctx, `DELETE FROM user_statuses WHERE user_id = $1 AND status = $2;`, userID, status)
	return err
}

func SetAdmin(ctx context.Context, pool *pgxpool.Pool, targetPlayerID string, requestingPlayerID string, isAdmin bool) error {
	if targetPlayerID == requestingPlayerID {
		return ErrCannotTargetSelf
	}
	_, err := pool.Exec(ctx, `UPDATE users SET is_admin = $1 WHERE player_id = $2;`, isAdmin, targetPlayerID)
	return err
}

func DeleteUser(ctx context.Context, pool *pgxpool.Pool, targetPlayerID string, requestingPlayerID string) error {
	if targetPlayerID == requestingPlayerID {
		return ErrCannotTargetSelf
	}
	return users_sql.DeleteUserByPlayerID(ctx, pool, targetPlayerID)
}
