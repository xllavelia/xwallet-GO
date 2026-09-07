package stocks_sql

import (
	"context"
	"errors"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func BuyStock(ctx context.Context, pool *pgxpool.Pool, userID int, symbol string, usdAmount float64, price float64) (float64, error) {
	if !IsValidSymbol(symbol) {
		return 0, ErrInvalidSymbol
	}

	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, err
	}

	quantity := usdAmount / price

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -usdAmount); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return 0, ErrInsufficientFunds
		}
		return 0, err
	}

	var existingQty, existingAvg float64
	err = tx.QueryRow(ctx, `SELECT quantity, avg_cost FROM stock_holdings WHERE user_id = $1 AND symbol = $2 FOR UPDATE;`, userID, symbol).Scan(&existingQty, &existingAvg)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	newQty := existingQty + quantity
	newAvgCost := existingAvg
	if newQty > 0 {
		newAvgCost = (existingQty*existingAvg + usdAmount) / newQty
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_holdings (user_id, symbol, quantity, avg_cost, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, symbol) DO UPDATE SET quantity = $3, avg_cost = $4, updated_at = now();
	`, userID, symbol, newQty, newAvgCost)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_history (user_id, symbol, operation_type, usd_amount, quantity, price)
		VALUES ($1, $2, 'buy', $3, $4, $5);
	`, userID, symbol, usdAmount, quantity, price)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return quantity, nil
}

func SellStock(ctx context.Context, pool *pgxpool.Pool, userID int, symbol string, usdAmount float64, price float64) (float64, float64, error) {
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return 0, 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	var existingQty, existingAvg float64
	err = tx.QueryRow(ctx, `SELECT quantity, avg_cost FROM stock_holdings WHERE user_id = $1 AND symbol = $2 FOR UPDATE;`, userID, symbol).Scan(&existingQty, &existingAvg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrNoHolding
		}
		return 0, 0, err
	}

	quantityToSell := usdAmount / price
	if quantityToSell > existingQty {
		quantityToSell = existingQty
		usdAmount = quantityToSell * price
	}
	if quantityToSell <= 0 {
		return 0, 0, ErrNoHolding
	}

	remainingQty := existingQty - quantityToSell
	realizedPnl := (price - existingAvg) * quantityToSell

	if remainingQty <= 0.00000001 {
		_, err = tx.Exec(ctx, `DELETE FROM stock_holdings WHERE user_id = $1 AND symbol = $2;`, userID, symbol)
	} else {
		_, err = tx.Exec(ctx, `UPDATE stock_holdings SET quantity = $1, updated_at = now() WHERE user_id = $2 AND symbol = $3;`, remainingQty, userID, symbol)
	}
	if err != nil {
		return 0, 0, err
	}

	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, usdAmount); err != nil {
		return 0, 0, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stock_history (user_id, symbol, operation_type, usd_amount, quantity, price, realized_pnl)
		VALUES ($1, $2, 'sell', $3, $4, $5, $6);
	`, userID, symbol, usdAmount, quantityToSell, price, realizedPnl)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return quantityToSell, realizedPnl, nil
}
