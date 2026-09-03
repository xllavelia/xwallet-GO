package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func InsertPositionTx(ctx context.Context, tx pgx.Tx, p Position) (Position, error) {
	sqlQuery := `
	INSERT INTO positions (trade_id, user_id, coin, type, entry_price, leverage, amount, margin, fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target, status, funding_kind, funding_card_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'open', $14, $15)
	RETURNING id, opened_at;
	`
	err := tx.QueryRow(ctx, sqlQuery,
		p.TradeID, p.UserID, p.Coin, p.Type, p.EntryPrice, p.Leverage, p.Amount, p.Margin,
		p.Fees, p.FeesPaidByVoucher, p.LiqPrice, p.AutoClose, p.AutoCloseTarget, p.FundingKind, p.FundingCardID,
	).Scan(&p.ID, &p.OpenedAt)

	return p, err
}
