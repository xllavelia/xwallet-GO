package transfer_sql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

func generateReferenceCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

func ExecuteTransfer(ctx context.Context, pool *pgxpool.Pool, senderUserID int, recipientUserID int, amount float64) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var newSenderBalance float64
	err = tx.QueryRow(ctx, `
		UPDATE wallets SET balance = balance - $1, updated_at = now()
		WHERE user_id = $2 AND balance >= $1
		RETURNING balance;
	`, amount, senderUserID).Scan(&newSenderBalance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrInsufficientBalance
		}
		return 0, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE wallets SET balance = balance + $1, updated_at = now()
		WHERE user_id = $2;
	`, amount, recipientUserID)
	if err != nil {
		return 0, err
	}

	refCode, err := generateReferenceCode()
	if err != nil {
		return 0, err
	}

	var transferID int
	err = tx.QueryRow(ctx, `
		INSERT INTO transfers (sender_id, recipient_id, amount, status, reference_code)
		VALUES ($1, $2, $3, 'completed', $4)
		RETURNING id;
	`, senderUserID, recipientUserID, amount, refCode).Scan(&transferID)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	_ = time.Now() // (created_at читается отдельно через GetTransferDetail)
	return transferID, nil
}
