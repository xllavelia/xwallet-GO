package wallet_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetWalletByPlayerID(ctx context.Context, pool *pgxpool.Pool, playerID string) (Wallet, error) {
	sqlQuery := `
	SELECT w.user_id, w.balance, w.profit_24h, w.profit_7d, w.active_trades_count, w.win_rate
	FROM wallets w
	JOIN users u ON u.id = w.user_id
	WHERE u.player_id = $1;
	`
	var w Wallet
	err := pool.QueryRow(ctx, sqlQuery, playerID).Scan(
		&w.UserID, &w.Balance, &w.Profit24h, &w.Profit7d, &w.ActiveTradesCount, &w.WinRate,
	)

	return w, err
}
