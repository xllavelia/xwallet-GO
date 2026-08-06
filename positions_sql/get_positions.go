package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetOpenPositionsByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Position, error) {
	sqlQuery := `
	SELECT id, trade_id, user_id, coin, type, entry_price, close_price, leverage, amount, margin,
	       fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target,
	       pnl, pnl_percent, status, result, opened_at, closed_at, xp_awarded
	FROM positions WHERE user_id = $1 AND status = 'open' ORDER BY opened_at DESC;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		var p Position
		err := rows.Scan(&p.ID, &p.TradeID, &p.UserID, &p.Coin, &p.Type, &p.EntryPrice, &p.ClosePrice, &p.Leverage,
			&p.Amount, &p.Margin, &p.Fees, &p.FeesPaidByVoucher, &p.LiqPrice, &p.AutoClose, &p.AutoCloseTarget,
			&p.Pnl, &p.PnlPercent, &p.Status, &p.Result, &p.OpenedAt, &p.ClosedAt, &p.XpAwarded)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func GetAllOpenPositions(ctx context.Context, pool *pgxpool.Pool) ([]Position, error) {
	sqlQuery := `
	SELECT id, trade_id, user_id, coin, type, entry_price, close_price, leverage, amount, margin,
	       fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target,
	       pnl, pnl_percent, status, result, opened_at, closed_at, xp_awarded
	FROM positions WHERE status = 'open';
	`
	rows, err := pool.Query(ctx, sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		var p Position
		err := rows.Scan(&p.ID, &p.TradeID, &p.UserID, &p.Coin, &p.Type, &p.EntryPrice, &p.ClosePrice, &p.Leverage,
			&p.Amount, &p.Margin, &p.Fees, &p.FeesPaidByVoucher, &p.LiqPrice, &p.AutoClose, &p.AutoCloseTarget,
			&p.Pnl, &p.PnlPercent, &p.Status, &p.Result, &p.OpenedAt, &p.ClosedAt, &p.XpAwarded)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func GetPositionByID(ctx context.Context, pool *pgxpool.Pool, id int) (Position, error) {
	sqlQuery := `
	SELECT id, trade_id, user_id, coin, type, entry_price, close_price, leverage, amount, margin,
	       fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target,
	       pnl, pnl_percent, status, result, opened_at, closed_at, xp_awarded
	FROM positions WHERE id = $1;
	`
	var p Position
	err := pool.QueryRow(ctx, sqlQuery, id).Scan(&p.ID, &p.TradeID, &p.UserID, &p.Coin, &p.Type, &p.EntryPrice, &p.ClosePrice, &p.Leverage,
		&p.Amount, &p.Margin, &p.Fees, &p.FeesPaidByVoucher, &p.LiqPrice, &p.AutoClose, &p.AutoCloseTarget,
		&p.Pnl, &p.PnlPercent, &p.Status, &p.Result, &p.OpenedAt, &p.ClosedAt, &p.XpAwarded)
	return p, err
}

func GetClosedPositionsByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Position, error) {
	sqlQuery := `
	SELECT id, trade_id, user_id, coin, type, entry_price, close_price, leverage, amount, margin,
	       fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target,
	       pnl, pnl_percent, status, result, opened_at, closed_at, xp_awarded
	FROM positions WHERE user_id = $1 AND status = 'closed' ORDER BY closed_at DESC LIMIT 100;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		var p Position
		err := rows.Scan(&p.ID, &p.TradeID, &p.UserID, &p.Coin, &p.Type, &p.EntryPrice, &p.ClosePrice, &p.Leverage,
			&p.Amount, &p.Margin, &p.Fees, &p.FeesPaidByVoucher, &p.LiqPrice, &p.AutoClose, &p.AutoCloseTarget,
			&p.Pnl, &p.PnlPercent, &p.Status, &p.Result, &p.OpenedAt, &p.ClosedAt, &p.XpAwarded)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
