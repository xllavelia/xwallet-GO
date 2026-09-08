package rocket_sql

import (
	"context"
	"errors"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PlaceBet(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int, amount float64) (int, error) {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -amount); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return 0, ErrInsufficientFunds
		}
		return 0, err
	}

	var betID int
	err = tx.QueryRow(ctx, `
		INSERT INTO rocket_bets (round_id, user_id, amount, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id;
	`, roundID, userID, amount).Scan(&betID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, ErrAlreadyBet
		}
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return betID, nil
}

func CashOutBet(ctx context.Context, pool *pgxpool.Pool, userID int, roundID int, multiplier float64) (float64, error) {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var amount float64
	err = tx.QueryRow(ctx, `
		UPDATE rocket_bets SET status = 'cashed_out', cashout_multiplier = $1, payout = amount * $1
		WHERE user_id = $2 AND round_id = $3 AND status = 'active'
		RETURNING amount;
	`, multiplier, userID, roundID).Scan(&amount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNoActiveBet
		}
		return 0, err
	}

	payout := amount * multiplier
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, payout); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return payout, nil
}

func SettleLostBets(ctx context.Context, pool *pgxpool.Pool, roundID int) error {
	_, err := pool.Exec(ctx, `UPDATE rocket_bets SET status = 'lost' WHERE round_id = $1 AND status = 'active';`, roundID)
	return err
}
