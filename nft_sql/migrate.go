package nft_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS nft_collections (
	id INT PRIMARY KEY,
	name TEXT NOT NULL,
	theme TEXT NOT NULL,
	tagline TEXT NOT NULL,
	starts_at TIMESTAMPTZ NOT NULL,
	ends_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS nft_templates (
	id TEXT PRIMARY KEY,
	collection_id INT NOT NULL REFERENCES nft_collections(id),
	slot INT NOT NULL,
	name TEXT NOT NULL,
	rarity TEXT NOT NULL,
	art JSONB NOT NULL,
	base_price NUMERIC(14,2) NOT NULL CHECK (base_price > 0),
	income_hour NUMERIC(14,6) NOT NULL CHECK (income_hour >= 0),
	supply INT NOT NULL CHECK (supply > 0),
	minted INT NOT NULL DEFAULT 0 CHECK (minted >= 0 AND minted <= supply),
	UNIQUE (collection_id, slot)
);
CREATE INDEX IF NOT EXISTS nft_templates_rarity_idx ON nft_templates(rarity, collection_id);

CREATE TABLE IF NOT EXISTS nft_items (
	id BIGSERIAL PRIMARY KEY,
	template_id TEXT NOT NULL REFERENCES nft_templates(id),
	serial INT NOT NULL,
	owner_id INT NOT NULL,
	source TEXT NOT NULL,
	minted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	accrued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (template_id, serial)
);
CREATE INDEX IF NOT EXISTS nft_items_owner_idx ON nft_items(owner_id);

CREATE TABLE IF NOT EXISTS nft_packs (
	id TEXT PRIMARY KEY,
	collection_id INT NOT NULL REFERENCES nft_collections(id),
	code TEXT NOT NULL,
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	caption TEXT NOT NULL,
	slots JSONB NOT NULL,
	price NUMERIC(14,2) NOT NULL CHECK (price > 0),
	stock INT NOT NULL,
	sold INT NOT NULL DEFAULT 0 CHECK (sold >= 0 AND sold <= stock),
	UNIQUE (collection_id, code)
);

CREATE TABLE IF NOT EXISTS nft_listings (
	id BIGSERIAL PRIMARY KEY,
	item_id BIGINT NOT NULL UNIQUE REFERENCES nft_items(id) ON DELETE CASCADE,
	seller_id INT NOT NULL,
	price NUMERIC(14,2) NOT NULL CHECK (price > 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS nft_listings_created_idx ON nft_listings(created_at DESC);

CREATE TABLE IF NOT EXISTS nft_game_listings (
	day DATE NOT NULL,
	template_id TEXT NOT NULL REFERENCES nft_templates(id),
	mult NUMERIC(6,3) NOT NULL,
	stock INT NOT NULL,
	sold INT NOT NULL DEFAULT 0 CHECK (sold >= 0 AND sold <= stock),
	PRIMARY KEY (day, template_id)
);

CREATE TABLE IF NOT EXISTS nft_stats (
	user_id INT PRIMARY KEY,
	carry NUMERIC(18,6) NOT NULL DEFAULT 0,
	earned NUMERIC(14,2) NOT NULL DEFAULT 0,
	sold NUMERIC(14,2) NOT NULL DEFAULT 0,
	spent NUMERIC(14,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS nft_sales (
	id BIGSERIAL PRIMARY KEY,
	template_id TEXT NOT NULL,
	serial INT NOT NULL,
	seller_id INT,
	buyer_id INT,
	kind TEXT NOT NULL,
	price NUMERIC(14,2) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS nft_sales_created_idx ON nft_sales(created_at DESC);

CREATE TABLE IF NOT EXISTS nft_meta (
	key TEXT PRIMARY KEY,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Блокировка защищает схему при параллельном старте инстансов.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1735390077)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schemaSQL); err != nil {
		return err
	}
	// Разовая миграция: уже выпущенные шаблоны получают повышенную доходность.
	tag, err := tx.Exec(ctx, `INSERT INTO nft_meta(key) VALUES ('income_boost_6x') ON CONFLICT (key) DO NOTHING`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		for _, r := range Rarities {
			if _, err = tx.Exec(ctx, `
UPDATE nft_templates SET income_hour = ROUND(base_price * $1::numeric / 24, 6) WHERE rarity = $2`, r.Yield, r.Key); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
