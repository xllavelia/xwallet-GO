package empire_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateEmpireTables(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
CREATE TABLE IF NOT EXISTS empire_profiles(
    user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    level INT NOT NULL DEFAULT 1,
    xp NUMERIC(16, 2) NOT NULL DEFAULT 0,
    prestige_count INT NOT NULL DEFAULT 0,
    prestige_points INT NOT NULL DEFAULT 0,
    rp NUMERIC(14, 2) NOT NULL DEFAULT 0,
    total_earned NUMERIC(18, 2) NOT NULL DEFAULT 0,
    all_time_earned NUMERIC(18, 2) NOT NULL DEFAULT 0,
    total_spent NUMERIC(18, 2) NOT NULL DEFAULT 0,
    last_tick TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS empire_objects(
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    def_id VARCHAR(48) NOT NULL,
    custom_name VARCHAR(64),
    level INT NOT NULL DEFAULT 1,
    workers_hired INT NOT NULL DEFAULT 0,
    worker_xp NUMERIC(12, 2) NOT NULL DEFAULT 0,
    satisfaction NUMERIC(5, 2) NOT NULL DEFAULT 75,
    invested NUMERIC(16, 2) NOT NULL DEFAULT 0,
    lifetime_earned NUMERIC(18, 2) NOT NULL DEFAULT 0,
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS empire_objects_user_idx ON empire_objects(user_id);

CREATE TABLE IF NOT EXISTS empire_research(
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    direction VARCHAR(24) NOT NULL,
    level INT NOT NULL DEFAULT 0,
    PRIMARY KEY(user_id, direction)
);

-- Цены активов общие для всего сервера.
CREATE TABLE IF NOT EXISTS empire_asset_prices(
    asset_id VARCHAR(24) PRIMARY KEY,
    price NUMERIC(14, 2) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS empire_asset_history(
    id SERIAL PRIMARY KEY,
    asset_id VARCHAR(24) NOT NULL,
    price NUMERIC(14, 2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS empire_asset_history_asset_idx ON empire_asset_history(asset_id);

CREATE TABLE IF NOT EXISTS empire_asset_holdings(
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asset_id VARCHAR(24) NOT NULL,
    shares INT NOT NULL DEFAULT 0,
    avg_price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    dividends_earned NUMERIC(16, 2) NOT NULL DEFAULT 0,
    PRIMARY KEY(user_id, asset_id)
);

CREATE TABLE IF NOT EXISTS empire_contracts(
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id VARCHAR(24) NOT NULL,
    title VARCHAR(96) NOT NULL,
    metric VARCHAR(24) NOT NULL,
    target NUMERIC(18, 2) NOT NULL,
    progress NUMERIC(18, 2) NOT NULL DEFAULT 0,
    reward_money NUMERIC(16, 2) NOT NULL DEFAULT 0,
    reward_xp NUMERIC(14, 2) NOT NULL DEFAULT 0,
    reward_rp NUMERIC(14, 2) NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS empire_contracts_user_idx ON empire_contracts(user_id, status);

CREATE TABLE IF NOT EXISTS empire_boosters(
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    booster_type VARCHAR(24) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(user_id, booster_type)
);

-- Журнал событий для аналитики (покупки, апгрейды, продажи и т.д.).
CREATE TABLE IF NOT EXISTS empire_events(
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(24) NOT NULL,
    ref VARCHAR(64) NOT NULL DEFAULT '',
    amount NUMERIC(18, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS empire_events_user_idx ON empire_events(user_id, created_at DESC);

-- Дневная агрегация доходов для графиков аналитики.
CREATE TABLE IF NOT EXISTS empire_daily(
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day DATE NOT NULL,
    income NUMERIC(18, 2) NOT NULL DEFAULT 0,
    expenses NUMERIC(18, 2) NOT NULL DEFAULT 0,
    net NUMERIC(18, 2) NOT NULL DEFAULT 0,
    PRIMARY KEY(user_id, day)
);
`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}

// SeedEmpireAssets — стартовые цены активов (одинаковые для всех,
// дальше цены живут своей жизнью через random walk).
func SeedEmpireAssets(ctx context.Context, pool *pgxpool.Pool) error {
	for _, def := range EmpireAssets {
		_, err := pool.Exec(ctx, `
INSERT INTO empire_asset_prices (asset_id, price)
VALUES ($1, $2)
ON CONFLICT (asset_id) DO NOTHING;`, def.ID, def.BasePrice)
		if err != nil {
			return err
		}
	}
	return nil
}
