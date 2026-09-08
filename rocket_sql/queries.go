package rocket_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertRound(ctx context.Context, pool *pgxpool.Pool, crashPoint float64) (int, error) {
	var id int
	err := pool.QueryRow(ctx, `INSERT INTO rocket_rounds (crash_point) VALUES ($1) RETURNING id;`, crashPoint).Scan(&id)
	return id, err
}

func StartRound(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `UPDATE rocket_rounds SET started_at = now() WHERE id = $1;`, roundID)
	return err
}

func EndRound(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `UPDATE rocket_rounds SET ended_at = now() WHERE id = $1;`, roundID)
	return err
}

func GetRecentCrashPoints(ctx context.Context, pool *pgxpool.Pool, limit int) ([]float64, error) {
	rows, err := pool.Query(ctx, `SELECT crash_point FROM rocket_rounds WHERE ended_at IS NOT NULL ORDER BY id DESC LIMIT $1;`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []float64{}
	for rows.Next() {
		var cp float64
		if err := rows.Scan(&cp); err == nil {
			result = append(result, cp)
		}
	}
	return result, rows.Err()
}

type GlobalStats struct {
	TotalBets     int
	SuccessBets   int
	MaxMultiplier float64
}

func GetGlobalStats(ctx context.Context, pool *pgxpool.Pool) (GlobalStats, error) {
	var s GlobalStats
	err := pool.QueryRow(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'cashed_out') FROM rocket_bets;`).Scan(&s.TotalBets, &s.SuccessBets)
	if err != nil {
		return s, err
	}
	err = pool.QueryRow(ctx, `SELECT COALESCE(MAX(crash_point), 0) FROM rocket_rounds WHERE ended_at IS NOT NULL;`).Scan(&s.MaxMultiplier)
	return s, err
}

type UserStats struct {
	TotalBets   int
	WinBets     int
	TotalProfit float64
	MaxWin      float64
}

func GetUserStats(ctx context.Context, pool *pgxpool.Pool, userID int) (UserStats, error) {
	var s UserStats
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'cashed_out'),
			COALESCE(SUM(COALESCE(payout,0) - amount), 0),
			COALESCE(MAX(payout), 0)
		FROM rocket_bets WHERE user_id = $1;
	`, userID).Scan(&s.TotalBets, &s.WinBets, &s.TotalProfit, &s.MaxWin)
	return s, err
}

func GetActiveBet(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int) (Bet, error) {
	var b Bet
	err := pool.QueryRow(ctx, `
		SELECT id, round_id, user_id, amount, status, cashout_multiplier, payout, created_at
		FROM rocket_bets WHERE user_id = $1 AND round_id = $2;
	`, userID, roundID).Scan(&b.ID, &b.RoundID, &b.UserID, &b.Amount, &b.Status, &b.CashoutMultiplier, &b.Payout, &b.CreatedAt)
	return b, err
}
