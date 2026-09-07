package stocks_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetHoldings(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Holding, error) {
	rows, err := pool.Query(ctx, `SELECT symbol, quantity, avg_cost FROM stock_holdings WHERE user_id = $1 AND quantity > 0;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Holding{}
	for rows.Next() {
		var h Holding
		if err := rows.Scan(&h.Symbol, &h.Quantity, &h.AvgCost); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
