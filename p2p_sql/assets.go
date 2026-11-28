package p2p_sql

import (
	"context"
	"errors"
	"fmt"

	"xwallet-server/bankcards_sql"
	"xwallet-server/card_sql"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficientAsset = errors.New("insufficient balance for this asset")

var cryptoColumn = map[string]string{
	"BTC": "btc_amount",
	"ETH": "eth_amount",
	"SOL": "sol_amount",
	"TON": "ton_amount",
}

func moveCryptoOut(ctx context.Context, q bankcards_sql.Queryer, userID int, coin string, amount float64) error {
	col, ok := cryptoColumn[coin]
	if !ok {
		return fmt.Errorf("unsupported coin: %s", coin)
	}
	cardID, err := card_sql.LockCryptoCard(ctx, q, userID)
	if err != nil {
		if errors.Is(err, card_sql.ErrCryptoCardNotFound) {
			return ErrInsufficientAsset
		}
		return err
	}
	sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s - $1 WHERE id = $3 AND user_id = $2 AND %s >= $1 RETURNING %s;`, col, col, col, col)
	var newAmt float64
	err = q.QueryRow(ctx, sqlQuery, amount, userID, cardID).Scan(&newAmt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientAsset
		}
		return err
	}
	return nil
}

func moveCryptoIn(ctx context.Context, q bankcards_sql.Queryer, userID int, coin string, amount float64) error {
	col, ok := cryptoColumn[coin]
	if !ok {
		return fmt.Errorf("unsupported coin: %s", coin)
	}
	cardID, err := card_sql.EnsureCryptoCard(ctx, q, userID)
	if err != nil {
		return err
	}
	sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s + $1 WHERE id = $3 AND user_id = $2;`, col, col)
	tag, err := q.Exec(ctx, sqlQuery, amount, userID, cardID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return card_sql.ErrCryptoCardNotFound
	}
	return nil
}

func moveLavxOut(ctx context.Context, q bankcards_sql.Queryer, userID int, amount float64) error {
	var newBal float64
	err := q.QueryRow(ctx, `
		UPDATE wallets SET lavx_balance = lavx_balance - $1
		WHERE user_id = $2 AND lavx_balance >= $1
		RETURNING lavx_balance;
	`, amount, userID).Scan(&newBal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientAsset
		}
		return err
	}
	return nil
}

func moveLavxIn(ctx context.Context, q bankcards_sql.Queryer, userID int, amount float64) error {
	_, err := q.Exec(ctx, `UPDATE wallets SET lavx_balance = lavx_balance + $1 WHERE user_id = $2;`, amount, userID)
	return err
}

func moveStockOut(ctx context.Context, q bankcards_sql.Queryer, userID int, symbol string, quantity float64) error {
	var qty float64
	err := q.QueryRow(ctx, `SELECT quantity FROM stock_holdings WHERE user_id = $1 AND symbol = $2 FOR UPDATE;`, userID, symbol).Scan(&qty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientAsset
		}
		return err
	}
	if qty < quantity {
		return ErrInsufficientAsset
	}
	remaining := qty - quantity
	if remaining <= 0.00000001 {
		_, err = q.Exec(ctx, `DELETE FROM stock_holdings WHERE user_id = $1 AND symbol = $2;`, userID, symbol)
	} else {
		_, err = q.Exec(ctx, `UPDATE stock_holdings SET quantity = $1, updated_at = now() WHERE user_id = $2 AND symbol = $3;`, remaining, userID, symbol)
	}
	return err
}

func moveStockIn(ctx context.Context, q bankcards_sql.Queryer, userID int, symbol string, quantity float64, costBasisPerUnit float64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO stock_holdings (user_id, symbol, quantity, avg_cost, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, symbol) DO UPDATE
		SET quantity = COALESCE(stock_holdings.quantity, 0) + EXCLUDED.quantity,
		    avg_cost = CASE
		        WHEN COALESCE(stock_holdings.quantity, 0) + EXCLUDED.quantity > 0 THEN
		            (COALESCE(stock_holdings.quantity, 0) * COALESCE(stock_holdings.avg_cost, 0)
		                + EXCLUDED.quantity * EXCLUDED.avg_cost)
		            / (COALESCE(stock_holdings.quantity, 0) + EXCLUDED.quantity)
		        ELSE 0
		    END,
		    updated_at = now();
	`, userID, symbol, quantity, costBasisPerUnit)
	return err
}

func moveAssetOut(ctx context.Context, q bankcards_sql.Queryer, userID int, assetClass string, asset string, amount float64) error {
	switch assetClass {
	case "lavx":
		return moveLavxOut(ctx, q, userID, amount)
	case "crypto":
		return moveCryptoOut(ctx, q, userID, asset, amount)
	case "stock":
		return moveStockOut(ctx, q, userID, asset, amount)
	}
	return errors.New("unsupported asset class")
}

func moveAssetIn(ctx context.Context, q bankcards_sql.Queryer, userID int, assetClass string, asset string, amount float64, costBasisPerUnit float64) error {
	switch assetClass {
	case "lavx":
		return moveLavxIn(ctx, q, userID, amount)
	case "crypto":
		return moveCryptoIn(ctx, q, userID, asset, amount)
	case "stock":
		return moveStockIn(ctx, q, userID, asset, amount, costBasisPerUnit)
	}
	return errors.New("unsupported asset class")
}
