package card_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateCryptoCardsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS crypto_cards(
		id SERIAL PRIMARY KEY,
		card_number CHAR(16) NOT NULL UNIQUE,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		btc_amount NUMERIC(18, 8) NOT NULL DEFAULT 0,
		eth_amount NUMERIC(18, 8) NOT NULL DEFAULT 0,
		sol_amount NUMERIC(18, 8) NOT NULL DEFAULT 0,
		ton_amount NUMERIC(18, 8) NOT NULL DEFAULT 0,
		valid_thru DATE NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT now()
	);
	CREATE INDEX IF NOT EXISTS idx_crypto_cards_user ON crypto_cards (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
