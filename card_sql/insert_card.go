package card_sql

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func generateCardNumber() string {
	digits := "0123456789"
	b := make([]byte, 16)
	rand.Read(b)
	out := make([]byte, 16)
	for i, v := range b {
		out[i] = digits[int(v)%10]
	}
	return string(out)
}

func InsertCard(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	sqlQuery := `
	INSERT INTO crypto_cards (card_number, user_id, valid_thru)
	SELECT $1, $2, $3
	WHERE NOT EXISTS (SELECT 1 FROM crypto_cards WHERE user_id = $2);
	`
	validThru := time.Now().AddDate(2, 0, 0)
	_, err := pool.Exec(ctx, sqlQuery, generateCardNumber(), userID, validThru)

	return err
}
