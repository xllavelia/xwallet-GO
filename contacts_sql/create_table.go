package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateContactsTable(ctx context.Context, pool *pgxpool.Pool) error {
	sqlQuery := `
	CREATE TABLE IF NOT EXISTS contacts(
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		contact_user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT now(),

		UNIQUE(user_id, contact_user_id)
	);
	CREATE INDEX IF NOT EXISTS idx_contacts_user ON contacts (user_id);
	`
	_, err := pool.Exec(ctx, sqlQuery)

	return err
}
