package users_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoDeviceOnRecord = errors.New("no device id on record for this user")
var ErrDeviceBanned = errors.New("device banned")

type BanStatus struct {
	AccountBanned bool
	BanReason     string
	DeviceBanned  bool
}

func BanAccount(ctx context.Context, pool *pgxpool.Pool, playerID string, reason string) error {
	if reason == "" {
		reason = "unknown"
	}
	tag, err := pool.Exec(ctx, `UPDATE users SET banned = TRUE, ban_reason = $2 WHERE player_id = $1;`,
		playerID, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("user not found")
	}
	return nil
}

func BanDevice(ctx context.Context, pool *pgxpool.Pool, playerID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deviceID *string
	if err := tx.QueryRow(ctx, `SELECT device_id FROM users WHERE player_id = $1 FOR UPDATE;`, playerID).Scan(&deviceID); err != nil {
		return err
	}
	if deviceID == nil || *deviceID == "" {
		return ErrNoDeviceOnRecord
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO banned_devices (device_id) VALUES ($1) ON CONFLICT (device_id) DO NOTHING;`,
		*deviceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

func UpdateUserDevice(ctx context.Context, pool *pgxpool.Pool, playerID string, deviceID string) error {
	if deviceID == "" {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := UpdateUserDeviceTx(ctx, tx, playerID, deviceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpdateUserDeviceTx(ctx context.Context, tx pgx.Tx, playerID string, deviceID string) error {
	if deviceID == "" {
		return nil
	}
	var storedDeviceID *string
	if err := tx.QueryRow(ctx, `SELECT device_id FROM users WHERE player_id = $1 FOR UPDATE;`, playerID).Scan(&storedDeviceID); err != nil {
		return err
	}
	var deviceBanned bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM banned_devices WHERE device_id = $1)
		   OR EXISTS(SELECT 1 FROM banned_devices WHERE device_id = $2);
	`, storedDeviceID, deviceID).Scan(&deviceBanned); err != nil {
		return err
	}
	if deviceBanned {
		return ErrDeviceBanned
	}

	tag, err := tx.Exec(ctx, `UPDATE users SET device_id = $1 WHERE player_id = $2;`, deviceID, playerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("user not found")
	}
	return nil
}
