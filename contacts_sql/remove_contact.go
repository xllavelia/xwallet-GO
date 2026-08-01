package contacts_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RemoveContact(ctx context.Context, pool *pgxpool.Pool, userID int, contactUserID int) error {
	sqlQuery := `DELETE FROM contacts WHERE user_id = $1 AND contact_user_id = $2;`
	_, err := pool.Exec(ctx, sqlQuery, userID, contactUserID)

	return err
}
