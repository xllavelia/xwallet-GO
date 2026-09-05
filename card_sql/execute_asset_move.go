package card_sql

import (
	"context"
	"errors"
	"fmt"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficientAssetBalance = errors.New("insufficient balance")

func ExecuteAssetMove(ctx context.Context, q bankcards_sql.Queryer, fundingSource bankcards_sql.FundingSource, fromAsset string, toAsset string, fromAmount float64, toAmount float64) error {
	if fromAsset == "USDT" {
		if err := bankcards_sql.AdjustFundingBalance(ctx, q, fundingSource, -fromAmount); err != nil {
			if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
				return ErrInsufficientAssetBalance
			}
			return err
		}
	} else {
		column, ok := columnByCoin[fromAsset]
		if !ok {
			return fmt.Errorf("unsupported coin: %s", fromAsset)
		}
		sqlQuery := fmt.Sprintf(`
			UPDATE crypto_cards SET %s = %s - $1
			WHERE user_id = $2 AND %s >= $1
			RETURNING %s;
		`, column, column, column, column)
		var newAmount float64
		err := q.QueryRow(ctx, sqlQuery, fromAmount, fundingSource.UserID).Scan(&newAmount)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInsufficientAssetBalance
			}
			return err
		}
	}

	if toAsset == "USDT" {
		if err := bankcards_sql.AdjustFundingBalance(ctx, q, fundingSource, toAmount); err != nil {
			return err
		}
	} else {
		column, ok := columnByCoin[toAsset]
		if !ok {
			return fmt.Errorf("unsupported coin: %s", toAsset)
		}
		sqlQuery := fmt.Sprintf(`UPDATE crypto_cards SET %s = %s + $1 WHERE user_id = $2;`, column, column)
		if _, err := q.Exec(ctx, sqlQuery, toAmount, fundingSource.UserID); err != nil {
			return err
		}
	}

	return nil
}
