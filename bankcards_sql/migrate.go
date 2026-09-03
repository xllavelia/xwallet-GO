package bankcards_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateBankCardsSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS bank_cards(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		tier VARCHAR(16) NOT NULL,
		card_number CHAR(16) NOT NULL UNIQUE,
		balance NUMERIC(14, 2) NOT NULL DEFAULT 0,
		is_active_for_trading BOOLEAN NOT NULL DEFAULT false,
		last_lavx_grant_at TIMESTAMP,
		opened_at TIMESTAMP NOT NULL DEFAULT now(),

		CONSTRAINT bank_cards_tier_check CHECK (tier IN ('standard','classico','cobalt','astro','saint'))
	);
	CREATE INDEX IF NOT EXISTS idx_bank_cards_user ON bank_cards (user_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_bank_cards_one_active ON bank_cards (user_id) WHERE is_active_for_trading = true;

	ALTER TABLE positions ADD COLUMN IF NOT EXISTS funding_kind VARCHAR(8) NOT NULL DEFAULT 'wallet';
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS funding_card_id INT NULL REFERENCES bank_cards(id) ON DELETE SET NULL;
	ALTER TABLE positions ADD COLUMN IF NOT EXISTS cashback_awarded NUMERIC(14, 2) NOT NULL DEFAULT 0;

	ALTER TABLE transfers ADD COLUMN IF NOT EXISTS fee_amount NUMERIC(14, 2) NOT NULL DEFAULT 0;
	ALTER TABLE transfers ADD COLUMN IF NOT EXISTS sender_card_id INT NULL REFERENCES bank_cards(id) ON DELETE SET NULL;
	ALTER TABLE transfers ADD COLUMN IF NOT EXISTS recipient_card_id INT NULL REFERENCES bank_cards(id) ON DELETE SET NULL;

	CREATE TABLE IF NOT EXISTS voucher_claims_log(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		voucher_type VARCHAR(16) NOT NULL,
		amount NUMERIC(14, 2) NOT NULL,
		source VARCHAR(16) NOT NULL,
		claimed_at TIMESTAMP NOT NULL DEFAULT now()
	);
	`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
