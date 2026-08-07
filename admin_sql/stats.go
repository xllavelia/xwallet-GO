package admin_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PlatformStats struct {
	TotalUsers      int
	TotalBalance    float64
	TotalLavx       float64
	OpenPositions   int
	ClosedPositions int
	TotalVolume     float64
	TotalTransfers  int
	TransferVolume  float64
	TotalVouchers   int
	ActiveSubs      int
}

func GetPlatformStats(ctx context.Context, pool *pgxpool.Pool) (PlatformStats, error) {
	var s PlatformStats
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COALESCE(SUM(balance),0) FROM wallets),
			(SELECT COALESCE(SUM(lavx_balance),0) FROM wallets),
			(SELECT COUNT(*) FROM positions WHERE status='open'),
			(SELECT COUNT(*) FROM positions WHERE status='closed'),
			(SELECT COALESCE(SUM(amount),0) FROM positions),
			(SELECT COUNT(*) FROM transfers),
			(SELECT COALESCE(SUM(amount),0) FROM transfers),
			(SELECT COUNT(*) FROM user_vouchers),
			(SELECT COUNT(*) FROM prime_subscriptions WHERE tier IS NOT NULL AND expires_at > now());
	`).Scan(&s.TotalUsers, &s.TotalBalance, &s.TotalLavx, &s.OpenPositions, &s.ClosedPositions,
		&s.TotalVolume, &s.TotalTransfers, &s.TransferVolume, &s.TotalVouchers, &s.ActiveSubs)
	return s, err
}
