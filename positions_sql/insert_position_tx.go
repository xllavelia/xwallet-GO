package positions_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrDuplicateRequest = errors.New("this request was already processed")

func InsertPositionTx(ctx context.Context, tx pgx.Tx, p Position) (Position, error) {
	var clientReqID interface{}
	if p.ClientRequestID != "" {
		clientReqID = p.ClientRequestID
	}

	sqlQuery := `
	INSERT INTO positions (trade_id, user_id, coin, type, entry_price, leverage, amount, margin, fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target, status, funding_kind, funding_card_id, trade_mode, expires_at, payout_multiplier, client_request_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'open', $14, $15, $16, $17, $18, $19)
	RETURNING id, opened_at;
	`
	err := tx.QueryRow(ctx, sqlQuery,
		p.TradeID, p.UserID, p.Coin, p.Type, p.EntryPrice, p.Leverage, p.Amount, p.Margin,
		p.Fees, p.FeesPaidByVoucher, p.LiqPrice, p.AutoClose, p.AutoCloseTarget, p.FundingKind, p.FundingCardID,
		p.TradeMode, p.ExpiresAt, p.PayoutMultiplier, clientReqID,
	).Scan(&p.ID, &p.OpenedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Position{}, ErrDuplicateRequest
		}
		return Position{}, err
	}
	return p, nil
}
