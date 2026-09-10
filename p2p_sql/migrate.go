package p2p_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateP2PSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS p2p_merchants(
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		is_verified BOOLEAN NOT NULL DEFAULT false,
		deposit_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
		total_deals INT NOT NULL DEFAULT 0,
		completed_deals INT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS p2p_listings(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		side VARCHAR(4) NOT NULL CHECK (side IN ('buy','sell')),
		base_asset VARCHAR(10) NOT NULL,
		asset_class VARCHAR(10) NOT NULL CHECK (asset_class IN ('crypto','stock','lavx')),
		rate_usd NUMERIC(18,6) NOT NULL,
		min_amount_usd NUMERIC(14,2) NOT NULL,
		max_amount_usd NUMERIC(14,2) NOT NULL,
		remaining_base_amount NUMERIC(18,8) NOT NULL,
		payment_note TEXT,
		status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active','closed')),
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_p2p_listings_browse ON p2p_listings (status, side, asset_class);
	CREATE INDEX IF NOT EXISTS idx_p2p_listings_user ON p2p_listings (user_id);

	CREATE TABLE IF NOT EXISTS p2p_deals(
		id SERIAL PRIMARY KEY,
		listing_id INT NOT NULL REFERENCES p2p_listings(id) ON DELETE CASCADE,
		poster_user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		taker_user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		side VARCHAR(4) NOT NULL,
		base_asset VARCHAR(10) NOT NULL,
		asset_class VARCHAR(10) NOT NULL,
		base_amount NUMERIC(18,8) NOT NULL,
		quote_amount_usd NUMERIC(14,2) NOT NULL,
		rate_usd NUMERIC(18,6) NOT NULL,
		commission_usd NUMERIC(14,2) NOT NULL DEFAULT 0,
		status VARCHAR(16) NOT NULL DEFAULT 'awaiting_payment' CHECK (status IN ('awaiting_payment','completed','cancelled','expired')),
		expires_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_p2p_deals_poster ON p2p_deals (poster_user_id);
	CREATE INDEX IF NOT EXISTS idx_p2p_deals_taker ON p2p_deals (taker_user_id);
	CREATE INDEX IF NOT EXISTS idx_p2p_deals_expiry ON p2p_deals (status, expires_at);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
