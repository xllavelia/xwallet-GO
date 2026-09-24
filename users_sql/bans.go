package users_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoDeviceOnRecord = errors.New("no device id on record for this user")

type BanStatus struct {
	AccountBanned bool
	BanReason     string
	DeviceBanned  bool
}

func BanAccount(ctx context.Context, pool *pgxpool.Pool, playerID string) error {
	tag, err := pool.Exec(ctx, `UPDATE users SET banned = TRUE WHERE player_id = $1;`, playerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("user not found")
	}
	return nil
}

func BanDevice(ctx context.Context, pool *pgxpool.Pool, playerID string) error {
	var deviceID *string
	err := pool.QueryRow(ctx, `SELECT device_id FROM users WHERE player_id = $1;`, playerID).Scan(&deviceID)
	if err != nil {
		return err
	}
	if deviceID == nil || *deviceID == "" {
		return ErrNoDeviceOnRecord
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO banned_devices (device_id) VALUES ($1) ON CONFLICT (device_id) DO NOTHING;`,
		*deviceID)
	return err
}

// Одна кнопка снимает и бан аккаунта, и бан устройства.
func UnbanUser(ctx context.Context, pool *pgxpool.Pool, playerID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE users SET banned = FALSE, ban_reason = 'unknown' WHERE player_id = $1;`,
		playerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM banned_devices WHERE device_id = (SELECT device_id FROM users WHERE player_id = $1);`,
		playerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func GetBanStatus(ctx context.Context, pool *pgxpool.Pool, playerID string) (BanStatus, error) {
	var s BanStatus
	var deviceID *string
	err := pool.QueryRow(ctx,
		`SELECT banned, ban_reason, device_id FROM users WHERE player_id = $1;`,
		playerID).Scan(&s.AccountBanned, &s.BanReason, &deviceID)
	if err != nil {
		return BanStatus{}, err
	}
	if s.BanReason == "" {
		s.BanReason = "unknown"
	}
	if deviceID != nil && *deviceID != "" {
		_ = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM banned_devices WHERE device_id = $1);`,
			*deviceID).Scan(&s.DeviceBanned)
	}
	return s, nil
}

// Ошибка намеренно не возвращается: вход не должен падать из-за device-трекинга.
func UpdateUserDevice(ctx context.Context, pool *pgxpool.Pool, playerID string, deviceID string) {
	if deviceID == "" {
		return
	}
	pool.Exec(ctx, `UPDATE users SET device_id = $1 WHERE player_id = $2;`, deviceID, playerID)
}
