package wallet_sql

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NormalizeUSDAmount rounds a user-supplied USD amount to whole cents and
// reports whether the normalized amount is finite and positive.
func NormalizeUSDAmount(amount float64) (float64, bool) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, false
	}
	normalized := math.Round(amount*100) / 100
	if math.IsNaN(normalized) || math.IsInf(normalized, 0) || normalized <= 0 {
		return 0, false
	}
	return normalized, true
}

func AdjustBalance(ctx context.Context, pool *pgxpool.Pool, userID int, delta float64) error {
	sqlQuery := `
	UPDATE wallets
	SET balance = GREATEST(0, balance + $2), updated_at = now()
	WHERE user_id = $1;
	`
	_, err := pool.Exec(ctx, sqlQuery, userID, delta)

	return err
}

func GetBalanceByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (float64, error) {
	sqlQuery := `SELECT balance FROM wallets WHERE user_id = $1;`

	var balance float64
	err := pool.QueryRow(ctx, sqlQuery, userID).Scan(&balance)

	return balance, err
}
