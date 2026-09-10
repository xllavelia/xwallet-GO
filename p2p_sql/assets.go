package p2p_sql

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficientAsset = errors.New("insufficient balance for this asset")

var cryptoColumn = map[string]string{
	"BTC": "btc_amount",
	"ETH": "eth_amount",
	"SOL": "sol_amount",
	"TON": "ton_amount",
}

func randomCardNumber() string {
	digits := "0123456789"
	b := make([]byte, 16)
	rand.Read(b)
	out := make([]byte, 16)
	for i, v := range b {
		out[i] = digits[int(v)%10]
	}
	return string(out)
}

func moveCryptoOut(ctx context.Context, q bankcards_sql.Queryer, userID int, coin string, amount float64) error {
	col, ok := cryptoColumn[coin]
	if !ok {
		return fmt.Errorf("unsupported coin: %s", coin)
	}
	sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s - $1 WHERE user_id = $2 AND %s >= $1 RETURNING %s;`, col, col, col, col)
	var newAmt float64
	err := q.QueryRow(ctx, sqlQuery, amount, userID).Scan(&newAmt)
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
	_, _ = q.Exec(ctx, `
		INSERT INTO crypto_cards (card_number, user_id, valid_thru)
		SELECT $1, $2, now() + interval '2 years'
		WHERE NOT EXISTS (SELECT 1 FROM crypto_cards WHERE user_id = $2);
	`, randomCardNumber(), userID)
	sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s + $1 WHERE user_id = $2;`, col, col)
	_, err := q.Exec(ctx, sqlQuery, amount, userID)
	return err
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
	var existingQty, existingAvg float64
	err := q.QueryRow(ctx, `SELECT quantity, avg_cost FROM stock_holdings WHERE user_id = $1 AND symbol = $2 FOR UPDATE;`, userID, symbol).Scan(&existingQty, &existingAvg)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	newQty := existingQty + quantity
	newAvg := costBasisPerUnit
	if existingQty > 0 && newQty > 0 {
		newAvg = (existingQty*existingAvg + quantity*costBasisPerUnit) / newQty
	}
	_, err = q.Exec(ctx, `
		INSERT INTO stock_holdings (user_id, symbol, quantity, avg_cost, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, symbol) DO UPDATE SET quantity = $3, avg_cost = $4, updated_at = now();
	`, userID, symbol, newQty, newAvg)
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
