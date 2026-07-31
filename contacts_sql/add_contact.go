package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AddContact(ctx context.Context, pool *pgxpool.Pool, userID int, contactUserID int) error {
	sqlQuery := `
	INSERT INTO contacts (user_id, contact_user_id)
	VALUES ($1, $2)
	ON CONFLICT (user_id, contact_user_id) DO NOTHING;
	`
	_, err := pool.Exec(ctx, sqlQuery, userID, contactUserID)

	return err
}
