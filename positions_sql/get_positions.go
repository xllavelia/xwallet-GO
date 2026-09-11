package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

var fullColumns = `
	id, trade_id, user_id, coin, type, entry_price, close_price, leverage, amount, margin,
	fees, fees_paid_by_voucher, liq_price, auto_close, auto_close_target,
	pnl, pnl_percent, status, result, opened_at, closed_at, xp_awarded,
	funding_kind, funding_card_id, cashback_awarded, trade_mode, expires_at, payout_multiplier
`

func scanPosition(row interface {
	Scan(dest ...interface{}) error
}) (Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.TradeID, &p.UserID, &p.Coin, &p.Type, &p.EntryPrice, &p.ClosePrice, &p.Leverage,
		&p.Amount, &p.Margin, &p.Fees, &p.FeesPaidByVoucher, &p.LiqPrice, &p.AutoClose, &p.AutoCloseTarget,
		&p.Pnl, &p.PnlPercent, &p.Status, &p.Result, &p.OpenedAt, &p.ClosedAt, &p.XpAwarded,
		&p.FundingKind, &p.FundingCardID, &p.CashbackAwarded, &p.TradeMode, &p.ExpiresAt, &p.PayoutMultiplier)
	return p, err
}

func GetOpenPositionsByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Position, error) {
	rows, err := pool.Query(ctx, "SELECT "+fullColumns+" FROM positions WHERE user_id = $1 AND status = 'open' ORDER BY opened_at DESC;", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func GetAllOpenPositions(ctx context.Context, pool *pgxpool.Pool) ([]Position, error) {
	rows, err := pool.Query(ctx, "SELECT "+fullColumns+" FROM positions WHERE status = 'open';")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func GetPositionByID(ctx context.Context, pool *pgxpool.Pool, id int) (Position, error) {
	row := pool.QueryRow(ctx, "SELECT "+fullColumns+" FROM positions WHERE id = $1;", id)
	return scanPosition(row)
}

func GetClosedPositionsByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Position, error) {
	rows, err := pool.Query(ctx, "SELECT "+fullColumns+" FROM positions WHERE user_id = $1 AND status = 'closed' ORDER BY closed_at DESC LIMIT 100;", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Position
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func GetExpiredOpenTimeTradeIDs(ctx context.Context, pool *pgxpool.Pool) ([]int, error) {
	rows, err := pool.Query(ctx, `SELECT id FROM positions WHERE trade_mode='time' AND status='open' AND expires_at < now();`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}
