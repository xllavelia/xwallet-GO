package transfer_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTransferNotFound = errors.New("transfer not found")
var ErrForbidden = errors.New("forbidden")

type TransferDetail struct {
	ID                   int
	Direction            string
	Amount               float64
	CounterpartyUsername string
	CounterpartyPlayerID string
	ReferenceCode        string
	Status               string
	CreatedAt            time.Time
}

func GetTransferDetail(ctx context.Context, pool *pgxpool.Pool, transferID int, requestingUserID int) (TransferDetail, error) {
	sqlQuery := `
	SELECT t.id, t.sender_id, t.recipient_id,
	       su.username, su.player_id,
	       ru.username, ru.player_id,
	       t.amount, t.status, t.reference_code, t.created_at
	FROM transfers t
	JOIN users su ON su.id = t.sender_id
	JOIN users ru ON ru.id = t.recipient_id
	WHERE t.id = $1;
	`

	var senderID, recipientID int
	var senderName, senderPID, recipientName, recipientPID string
	var d TransferDetail

	err := pool.QueryRow(ctx, sqlQuery, transferID).Scan(
		&d.ID, &senderID, &recipientID,
		&senderName, &senderPID, &recipientName, &recipientPID,
		&d.Amount, &d.Status, &d.ReferenceCode, &d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransferDetail{}, ErrTransferNotFound
		}
		return TransferDetail{}, err
	}

	if senderID == requestingUserID {
		d.Direction = "send"
		d.CounterpartyUsername = recipientName
		d.CounterpartyPlayerID = recipientPID
	} else if recipientID == requestingUserID {
		d.Direction = "receive"
		d.CounterpartyUsername = senderName
		d.CounterpartyPlayerID = senderPID
	} else {
		return TransferDetail{}, ErrForbidden
	}

	return d, nil
}
