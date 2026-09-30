package open_city_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateOpenCitySchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
CREATE TABLE IF NOT EXISTS open_city_state(
	user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	doc_balance DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (doc_balance >= 0),
	level INT NOT NULL DEFAULT 1,
	xp INT NOT NULL DEFAULT 0,
	health INT NOT NULL DEFAULT 100,
	max_health INT NOT NULL DEFAULT 100,
	pos_x DOUBLE PRECISION NOT NULL DEFAULT 0,
	pos_y DOUBLE PRECISION NOT NULL DEFAULT 0,
	location VARCHAR(64) NOT NULL DEFAULT 'downtown',
	base_progress JSONB NOT NULL DEFAULT '{}',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_open_city_state_location
	ON open_city_state (location);

CREATE TABLE IF NOT EXISTS open_city_inventory(
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	item_id VARCHAR(64) NOT NULL,
	qty INT NOT NULL DEFAULT 1 CHECK (qty >= 0),
	PRIMARY KEY (user_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_open_city_inventory_item
	ON open_city_inventory (item_id);

CREATE TABLE IF NOT EXISTS open_city_quests(
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	quest_id VARCHAR(64) NOT NULL,
	status VARCHAR(24) NOT NULL DEFAULT 'active',
	progress JSONB NOT NULL DEFAULT '{}',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, quest_id)
);

-- Прокачка: очки навыков, вложенные очки (по StatDefs),
-- экипированное оружие. Отдельные колонки, а не base_progress:
-- их меняет только сервер, клиент лишь читает.
ALTER TABLE open_city_state
	ADD COLUMN IF NOT EXISTS skill_points INT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS stats JSONB NOT NULL DEFAULT '{}',
	ADD COLUMN IF NOT EXISTS equipped_weapon VARCHAR(64) NOT NULL DEFAULT '';
`

	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
