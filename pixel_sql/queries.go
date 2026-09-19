package pixel_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InsertRound(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var id int
	err := pool.QueryRow(ctx, `INSERT INTO pixel_rounds DEFAULT VALUES RETURNING id;`).Scan(&id)
	return id, err
}
func StartRound(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `UPDATE pixel_rounds SET started_at = now() WHERE id = $1;`, roundID)
	return err
}
func EndRound(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `UPDATE pixel_rounds SET ended_at = now() WHERE id = $1;`, roundID)
	return err
}

type GlobalStats struct {
	TotalGames   int
	SuccessGames int
	MaxPayout    float64
}

func GetGlobalStats(ctx context.Context, pool *pgxpool.Pool) (GlobalStats, error) {
	var s GlobalStats
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status IN ('cashed_out','expired'))
		FROM pixel_boards WHERE status != 'active';
	`).Scan(&s.TotalGames, &s.SuccessGames)
	if err != nil {
		return s, err
	}
	err = pool.QueryRow(ctx, `SELECT COALESCE(MAX(payout), 0) FROM pixel_boards WHERE payout IS NOT NULL;`).Scan(&s.MaxPayout)
	return s, err
}

type UserStats struct {
	TotalCellsOpened int
	SafeCellsOpened  int
	TotalGames       int
	GamesWon         int
	TotalProfit      float64
	MaxWin           float64
}

func GetUserStats(ctx context.Context, pool *pgxpool.Pool, userID int) (UserStats, error) {
	var s UserStats
	err := pool.QueryRow(ctx, `
		SELECT total_cells_opened, safe_cells_opened, total_games, games_won, total_profit
		FROM pixel_user_stats WHERE user_id = $1;
	`, userID).Scan(&s.TotalCellsOpened, &s.SafeCellsOpened, &s.TotalGames, &s.GamesWon, &s.TotalProfit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserStats{}, nil
		}
		return UserStats{}, err
	}
	pool.QueryRow(ctx, `SELECT COALESCE(MAX(payout - amount), 0) FROM pixel_boards WHERE user_id = $1 AND status = 'cashed_out';`, userID).Scan(&s.MaxWin)
	return s, nil
}
