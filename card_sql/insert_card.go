package card_sql

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
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

const cryptoCardAdvisoryLockNamespace = 1735390080

func LockCryptoCard(ctx context.Context, q bankcards_sql.Queryer, userID int) (int, error) {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock($1::integer, $2::integer);`, cryptoCardAdvisoryLockNamespace, userID); err != nil {
		return 0, err
	}
	return findCryptoCardForUpdate(ctx, q, userID)
}

func EnsureCryptoCard(ctx context.Context, q bankcards_sql.Queryer, userID int) (int, error) {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock($1::integer, $2::integer);`, cryptoCardAdvisoryLockNamespace, userID); err != nil {
		return 0, err
	}
	cardID, err := findCryptoCardForUpdate(ctx, q, userID)
	if err == nil {
		return cardID, nil
	}
	if !errors.Is(err, ErrCryptoCardNotFound) {
		return 0, err
	}

	err = q.QueryRow(ctx, `
		INSERT INTO crypto_cards (card_number, user_id, valid_thru)
		SELECT $1, $2, $3
		WHERE NOT EXISTS (SELECT 1 FROM crypto_cards WHERE user_id = $2)
		RETURNING id;
	`, generateCardNumber(), userID, time.Now().AddDate(2, 0, 0)).Scan(&cardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return findCryptoCardForUpdate(ctx, q, userID)
	}
	return cardID, err
}

func findCryptoCardForUpdate(ctx context.Context, q bankcards_sql.Queryer, userID int) (int, error) {
	var cardID int
	err := q.QueryRow(ctx, `
		SELECT id FROM crypto_cards
		WHERE user_id = $1
		ORDER BY id ASC
		LIMIT 1
		FOR UPDATE;
	`, userID).Scan(&cardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrCryptoCardNotFound
	}
	return cardID, err
}

func InsertCard(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := EnsureCryptoCard(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
