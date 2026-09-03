package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func ClosePositionTx(ctx context.Context, tx pgx.Tx, id int, closePrice float64, pnl float64, pnlPercent float64, result string, cashback float64) error {
	sqlQuery := `
	UPDATE positions
	SET close_price = $2, pnl = $3, pnl_percent = $4, result = $5, status = 'closed', closed_at = now(), cashback_awarded = $6
	WHERE id = $1 AND status = 'open';
	`
	_, err := tx.Exec(ctx, sqlQuery, id, closePrice, pnl, pnlPercent, result, cashback)
	return err
}
