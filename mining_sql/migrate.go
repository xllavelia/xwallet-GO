package mining_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateMiningSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS mining_profile (
		user_id              INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		energy               DOUBLE PRECISION NOT NULL DEFAULT 200,
		max_energy           DOUBLE PRECISION NOT NULL DEFAULT 200,
		extra_slots          INT NOT NULL DEFAULT 0,
		lifetime_earned      DOUBLE PRECISION NOT NULL DEFAULT 0,
		lifetime_spent       DOUBLE PRECISION NOT NULL DEFAULT 0,
		lifetime_energy_used DOUBLE PRECISION NOT NULL DEFAULT 0,
		last_tick_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
		created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS mining_servers (
		id                 BIGSERIAL PRIMARY KEY,
		user_id            INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		catalog_id         VARCHAR(32) NOT NULL,
		rank               INT NOT NULL DEFAULT 1,
		awake_until        TIMESTAMPTZ,
		exhausted          BOOLEAN NOT NULL DEFAULT false,
		total_earned       DOUBLE PRECISION NOT NULL DEFAULT 0,
		total_energy_used  DOUBLE PRECISION NOT NULL DEFAULT 0,
		purchased_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
		last_tick_at       TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_mining_servers_user ON mining_servers (user_id);
	CREATE INDEX IF NOT EXISTS idx_mining_servers_user_catalog ON mining_servers (user_id, catalog_id);

	CREATE TABLE IF NOT EXISTS mining_buffs (
		user_id     INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		buff_type   VARCHAR(24) NOT NULL,
		expires_at  TIMESTAMPTZ NOT NULL,
		PRIMARY KEY (user_id, buff_type)
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
