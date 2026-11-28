package card_sql

import (
	"context"
	"errors"
	"fmt"
	"math"

	"xwallet-server/bankcards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientAssetBalance = errors.New("insufficient balance")
var ErrCryptoCardNotFound = errors.New("crypto card not found")
var ErrInvalidAssetAmount = errors.New("asset amounts must be finite and positive")

func ExecuteAssetMove(ctx context.Context, pool *pgxpool.Pool, userID int, fromAsset string, toAsset string, fromAmount float64, toAmount float64) error {
	fromColumn, fromIsCoin := columnByCoin[fromAsset]
	toColumn, toIsCoin := columnByCoin[toAsset]
	if fromAsset != "USDT" && !fromIsCoin {
		return fmt.Errorf("unsupported coin: %s", fromAsset)
	}
	if toAsset != "USDT" && !toIsCoin {
		return fmt.Errorf("unsupported coin: %s", toAsset)
	}
	if fromAsset == toAsset || !validAssetAmount(fromAmount) || !validAssetAmount(toAmount) {
		return ErrInvalidAssetAmount
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var lockedUserID int
	// NO KEY UPDATE сериализует смену funding-карты, но не блокирует FK-проверки P2P.
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR NO KEY UPDATE;`, userID).Scan(&lockedUserID); err != nil {
		return err
	}

	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := bankcards_sql.LockFundingSource(ctx, tx, fundingSource); err != nil {
		return err
	}

	var cardID int
	if fromIsCoin || toIsCoin {
		err := tx.QueryRow(ctx, `
			SELECT id FROM crypto_cards
			WHERE user_id = $1
			ORDER BY id ASC
			LIMIT 1
			FOR UPDATE;
		`, userID).Scan(&cardID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCryptoCardNotFound
			}
			return err
		}
	}

	if fromAsset == "USDT" {
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -fromAmount); err != nil {
			if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
				return ErrInsufficientAssetBalance
			}
			return err
		}
	} else {
		sqlQuery := fmt.Sprintf(`
			UPDATE crypto_cards SET %s = %s - $1
			WHERE id = $3 AND user_id = $2 AND %s >= $1;
		`, fromColumn, fromColumn, fromColumn)
		tag, err := tx.Exec(ctx, sqlQuery, fromAmount, userID, cardID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrInsufficientAssetBalance
		}
	}

	if toAsset == "USDT" {
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, toAmount); err != nil {
			return err
		}
	} else {
		sqlQuery := fmt.Sprintf(`
			UPDATE crypto_cards SET %s = %s + $1
			WHERE id = $3 AND user_id = $2;
		`, toColumn, toColumn)
		tag, err := tx.Exec(ctx, sqlQuery, toAmount, userID, cardID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrCryptoCardNotFound
		}
	}

	return tx.Commit(ctx)
}

func validAssetAmount(amount float64) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount > 0
}
