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

	ALTER TABLE bank_cards ADD COLUMN IF NOT EXISTS name VARCHAR(32);
	ALTER TABLE bank_cards ADD COLUMN IF NOT EXISTS design_id INT;
	ALTER TABLE bank_cards ADD COLUMN IF NOT EXISTS design_key VARCHAR(32);
	ALTER TABLE bank_cards ADD COLUMN IF NOT EXISTS custom JSONB;
	WITH ordered AS (
		SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY opened_at ASC, id ASC) AS n
		FROM bank_cards
	)
	UPDATE bank_cards bc
	SET name = COALESCE(bc.name, 'Account ' || ordered.n::text),
		design_id = COALESCE(bc.design_id, ((ordered.n - 1) % 4)::int)
	FROM ordered WHERE bc.id = ordered.id AND (bc.name IS NULL OR bc.design_id IS NULL);
	-- Осталось четыре базовых дизайна (0–3): кошелёк и три основных счёта.
	UPDATE bank_cards SET design_id = design_id % 4 WHERE design_id IS NOT NULL AND design_id > 3;
	UPDATE bank_cards SET tier = 'standard' WHERE tier <> 'standard';
	ALTER TABLE bank_cards ALTER COLUMN tier SET DEFAULT 'standard';
	ALTER TABLE bank_cards ALTER COLUMN name SET NOT NULL;
	ALTER TABLE bank_cards ALTER COLUMN design_id SET NOT NULL;
	ALTER TABLE bank_cards DROP CONSTRAINT IF EXISTS bank_cards_design_check;
	ALTER TABLE bank_cards ADD CONSTRAINT bank_cards_design_check CHECK (design_id BETWEEN 0 AND 3);
	DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bank_cards_name_check' AND conrelid = 'bank_cards'::regclass) THEN
			ALTER TABLE bank_cards ADD CONSTRAINT bank_cards_name_check CHECK (char_length(name) BETWEEN 1 AND 32);
		END IF;
	END $$;

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
