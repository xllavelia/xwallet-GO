package users_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maintenanceKey = "maintenance"
const maintenanceNoTime = "notime"

func SetMaintenance(ctx context.Context, pool *pgxpool.Pool, until *time.Time) error {
	value := maintenanceNoTime
	if until != nil {
		value = until.UTC().Format(time.RFC3339)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();`,
		maintenanceKey, value)
	return err
}

func ClearMaintenance(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DELETE FROM app_settings WHERE key = $1;`, maintenanceKey)
	return err
}

func GetMaintenance(ctx context.Context, pool *pgxpool.Pool) (bool, *time.Time, error) {
	var value *string
	err := pool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1;`,
		maintenanceKey).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if value == nil || *value == "" || *value == maintenanceNoTime {
		return true, nil, nil
	}
	until, parseErr := time.Parse(time.RFC3339, *value)
	if parseErr != nil {
		return true, nil, nil
	}
	if until.Before(time.Now()) {
		return false, nil, nil
	}
	return true, &until, nil
}
