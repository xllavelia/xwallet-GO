package ticket_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateTicketSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `CREATE TABLE IF NOT EXISTS ticket_packs (
	id SERIAL PRIMARY KEY,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	rarity TEXT NOT NULL,
	price NUMERIC(14,2) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tickets (
	id SERIAL PRIMARY KEY,
	pack_id INT NOT NULL REFERENCES ticket_packs(id) ON DELETE CASCADE,
	user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	rarity TEXT NOT NULL,
	value NUMERIC(14,2) NOT NULL,
	idx INT NOT NULL,
	revealed BOOLEAN NOT NULL DEFAULT FALSE,
	revealed_at TIMESTAMPTZ,
	UNIQUE(pack_id, idx)
);
CREATE INDEX IF NOT EXISTS idx_tickets_user_rarity ON tickets(user_id, rarity, revealed);
CREATE TABLE IF NOT EXISTS ticket_user_stats (
	user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	packs_opened INT NOT NULL DEFAULT 0,
	tickets_opened INT NOT NULL DEFAULT 0,
	total_spent NUMERIC(14,2) NOT NULL DEFAULT 0,
	total_won NUMERIC(14,2) NOT NULL DEFAULT 0,
	best_ticket NUMERIC(14,2) NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`
	_, err := pool.Exec(ctx, sqlQuery)
	return err
}
