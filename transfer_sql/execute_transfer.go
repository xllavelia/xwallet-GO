package transfer_sql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"strings"

	"xwallet-server/bankcards_sql"

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

func ExecuteTransfer(ctx context.Context, pool *pgxpool.Pool, senderUserID int, recipientUserID int, amount float64) (int, float64, error) {
	if err := bankcards_sql.ValidateCardAmount(amount); err != nil {
		return 0, 0, err
	}
	amount = math.Round(amount*100) / 100
	// Единое округление сохраняет равенство списания, зачисления и комиссии.
	fee := math.Round(amount) / 100
	totalDebit := amount + fee

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	// Порядок блокировок исключает дедлок и смену источника денег во время перевода.
	first, second := senderUserID, recipientUserID
	if first > second {
		first, second = second, first
	}
	for _, userID := range []int{first, second} {
		var id int
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE;`, userID).Scan(&id); err != nil {
			return 0, 0, err
		}
	}
	senderSource, err := bankcards_sql.ResolveAnyCard(ctx, tx, senderUserID)
	if err != nil {
		return 0, 0, err
	}
	recipientSource, err := bankcards_sql.ResolveAnyCard(ctx, tx, recipientUserID)
	if err != nil {
		return 0, 0, err
	}

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, senderSource, -totalDebit); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return 0, 0, ErrInsufficientBalance
		}
		return 0, 0, err
	}
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, recipientSource, amount); err != nil {
		return 0, 0, err
	}

	refCode, err := generateReferenceCode()
	if err != nil {
		return 0, 0, err
	}

	var senderCardID, recipientCardID interface{}
	if senderSource.Kind == "card" {
		senderCardID = senderSource.CardID
	}
	if recipientSource.Kind == "card" {
		recipientCardID = recipientSource.CardID
	}

	var transferID int
	err = tx.QueryRow(ctx, `
		INSERT INTO transfers (sender_id, recipient_id, amount, status, reference_code, fee_amount, sender_card_id, recipient_card_id)
		VALUES ($1, $2, $3, 'completed', $4, $5, $6, $7)
		RETURNING id;
	`, senderUserID, recipientUserID, amount, refCode, fee, senderCardID, recipientCardID).Scan(&transferID)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}

	return transferID, fee, nil
}
