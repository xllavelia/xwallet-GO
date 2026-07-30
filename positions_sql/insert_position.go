package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertPosition(ctx context.Context, pool *pgxpool.Pool, p Position) (Position, error) {
	sqlQuery := `
	INSERT INTO positions (trade_id, user_id, coin, type, entry_price, leverage, amount, margin, fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target, status)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'open')
	RETURNING id, opened_at;
	`
	err := pool.QueryRow(ctx, sqlQuery,
		p.TradeID, p.UserID, p.Coin, p.Type, p.EntryPrice, p.Leverage, p.Amount, p.Margin,
		p.Fees, p.FeesPaidByVoucher, p.LiqPrice, p.AutoClose, p.AutoCloseTarget,
	).Scan(&p.ID, &p.OpenedAt)

	return p, err
}
