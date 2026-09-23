package flip_sql

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"xwallet-server/bankcards_sql"
)

const ChoiceRed = "red"
const ChoiceBlack = "black"

const BettingDuration = 10 * time.Second
const FlipDuration = 6 * time.Second

var PayoutMultiplier = 1.90

var ErrAlreadyBet = errors.New("you already placed a bet for this round")
var ErrNotBettingPhase = errors.New("betting is closed")
var ErrInsufficientFunds = errors.New("insufficient balance")
var ErrInvalidChoice = errors.New("invalid choice")
var ErrInvalidAmount = errors.New("invalid amount")
var ErrRoundResolved = errors.New("round is already resolved")

type Bet struct {
	ID        int
	RoundID   int
	UserID    int
	Amount    float64
	Choice    string
	Status    string
	Payout    *float64
	CreatedAt time.Time
}

type GlobalStats struct {
	TotalBets int
	WonBets   int
	MaxPayout float64
}

type UserStats struct {
	TotalBets   int
	WonBets     int
	TotalProfit float64
	MaxWin      float64
}

func MigrateFlipSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS flip_rounds (
			id SERIAL PRIMARY KEY,
			status VARCHAR(12) NOT NULL DEFAULT 'betting',
			outcome VARCHAR(5),
			betting_ends_at TIMESTAMP NOT NULL,
			flipping_started_at TIMESTAMP,
			ended_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT now(),
			CONSTRAINT flip_round_status_check
				CHECK (status IN ('betting', 'flipping', 'settled')),
			CONSTRAINT flip_round_outcome_check
				CHECK (outcome IS NULL OR outcome IN ('red', 'black'))
		);

		CREATE TABLE IF NOT EXISTS flip_bets (
			id SERIAL PRIMARY KEY,
			round_id INT NOT NULL REFERENCES flip_rounds(id) ON DELETE CASCADE,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
			choice VARCHAR(5) NOT NULL,
			status VARCHAR(12) NOT NULL DEFAULT 'active',
			payout NUMERIC(14,2),
			created_at TIMESTAMP NOT NULL DEFAULT now(),
			CONSTRAINT flip_bet_choice_check CHECK (choice IN ('red', 'black')),
			CONSTRAINT flip_bet_status_check CHECK (status IN ('active', 'won', 'lost')),
			UNIQUE(round_id, user_id)
		);

		CREATE INDEX IF NOT EXISTS idx_flip_bets_round ON flip_bets(round_id);
		CREATE INDEX IF NOT EXISTS idx_flip_bets_user ON flip_bets(user_id);
		CREATE INDEX IF NOT EXISTS idx_flip_rounds_finished ON flip_rounds(ended_at DESC);
	`)
	return err
}

func CreateRound(ctx context.Context, pool *pgxpool.Pool, bettingEndsAt time.Time) (int, error) {
	var roundID int
	err := pool.QueryRow(ctx, `
		INSERT INTO flip_rounds (betting_ends_at)
		VALUES ($1)
		RETURNING id;
	`, bettingEndsAt).Scan(&roundID)
	return roundID, err
}

func GetLastOutcome(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var outcome string
	err := pool.QueryRow(ctx, `
		SELECT outcome
		FROM flip_rounds
		WHERE outcome IS NOT NULL
		ORDER BY id DESC
		LIMIT 1;
	`).Scan(&outcome)

	if errors.Is(err, pgx.ErrNoRows) {
		return ChoiceBlack, nil
	}
	return outcome, err
}

func GetRecentOutcomes(ctx context.Context, pool *pgxpool.Pool, limit int) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT outcome
		FROM flip_rounds
		WHERE status = 'settled'
		ORDER BY id DESC
		LIMIT $1;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outcomes := []string{}
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

func GetBetForRound(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int) (Bet, error) {
	var bet Bet
	err := pool.QueryRow(ctx, `
		SELECT id, round_id, user_id, amount, choice, status, payout, created_at
		FROM flip_bets
		WHERE user_id = $1 AND round_id = $2;
	`, userID, roundID).Scan(
		&bet.ID,
		&bet.RoundID,
		&bet.UserID,
		&bet.Amount,
		&bet.Choice,
		&bet.Status,
		&bet.Payout,
		&bet.CreatedAt,
	)
	return bet, err
}

func GetGlobalStats(ctx context.Context, pool *pgxpool.Pool) (GlobalStats, error) {
	var stats GlobalStats
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'won'),
			COALESCE(MAX(payout), 0)
		FROM flip_bets
		WHERE status IN ('won', 'lost');
	`).Scan(&stats.TotalBets, &stats.WonBets, &stats.MaxPayout)
	return stats, err
}

func GetUserStats(ctx context.Context, pool *pgxpool.Pool, userID int) (UserStats, error) {
	var stats UserStats
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'won'),
			COALESCE(SUM(COALESCE(payout, 0) - amount), 0),
			COALESCE(MAX(payout), 0)
		FROM flip_bets
		WHERE user_id = $1
			AND status IN ('won', 'lost');
	`, userID).Scan(
		&stats.TotalBets,
		&stats.WonBets,
		&stats.TotalProfit,
		&stats.MaxWin,
	)
	return stats, err
}

func PlaceBet(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int, amount float64, choice string) (Bet, error) {
	var bet Bet

	if choice != ChoiceRed && choice != ChoiceBlack {
		return bet, ErrInvalidChoice
	}

	amount = roundMoney(amount)
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return bet, ErrInvalidAmount
	}

	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return bet, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return bet, err
	}
	defer tx.Rollback(ctx)

	var status string
	var bettingEndsAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT status, betting_ends_at
		FROM flip_rounds
		WHERE id = $1
		FOR UPDATE;
	`, roundID).Scan(&status, &bettingEndsAt)
	if err != nil {
		return bet, err
	}

	if status != "betting" || !time.Now().Before(bettingEndsAt) {
		return bet, ErrNotBettingPhase
	}

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -amount); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return bet, ErrInsufficientFunds
		}
		return bet, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO flip_bets (round_id, user_id, amount, choice)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at;
	`, roundID, userID, amount, choice).Scan(&bet.ID, &bet.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return bet, ErrAlreadyBet
		}
		return bet, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bet, err
	}

	bet.RoundID = roundID
	bet.UserID = userID
	bet.Amount = amount
	bet.Choice = choice
	bet.Status = "active"
	return bet, nil
}

func ResolveRound(ctx context.Context, pool *pgxpool.Pool, roundID int, outcome string) error {
	if outcome != ChoiceRed && outcome != ChoiceBlack {
		return ErrInvalidChoice
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var roundStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM flip_rounds
		WHERE id = $1
		FOR UPDATE;
	`, roundID).Scan(&roundStatus)
	if err != nil {
		return err
	}
	if roundStatus != "betting" {
		return ErrRoundResolved
	}

	rows, err := tx.Query(ctx, `
		SELECT id, round_id, user_id, amount, choice, status, payout, created_at
		FROM flip_bets
		WHERE round_id = $1 AND status = 'active'
		FOR UPDATE;
	`, roundID)
	if err != nil {
		return err
	}

	bets := []Bet{}
	for rows.Next() {
		var bet Bet
		if err := rows.Scan(
			&bet.ID,
			&bet.RoundID,
			&bet.UserID,
			&bet.Amount,
			&bet.Choice,
			&bet.Status,
			&bet.Payout,
			&bet.CreatedAt,
		); err != nil {
			rows.Close()
			return err
		}
		bets = append(bets, bet)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, bet := range bets {
		status := "lost"
		var payout *float64

		if bet.Choice == outcome {
			value := roundMoney(bet.Amount * PayoutMultiplier)
			fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, bet.UserID)
			if err != nil {
				return err
			}
			if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, value); err != nil {
				return err
			}
			status = "won"
			payout = &value
		}

		_, err = tx.Exec(ctx, `
			UPDATE flip_bets
			SET status = $1, payout = $2
			WHERE id = $3 AND status = 'active';
		`, status, payout, bet.ID)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE flip_rounds
		SET status = 'flipping',
			outcome = $1,
			flipping_started_at = now()
		WHERE id = $2;
	`, outcome, roundID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func FinishRound(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `
		UPDATE flip_rounds
		SET status = 'settled', ended_at = now()
		WHERE id = $1 AND status = 'flipping';
	`, roundID)
	return err
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}