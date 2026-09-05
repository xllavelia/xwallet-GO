package bankcards_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func ResolveFundingSource(ctx context.Context, q Queryer, userID int) (FundingSource, error) {
	var cardID int
	err := q.QueryRow(ctx, `SELECT id FROM bank_cards WHERE user_id = $1 AND is_active_for_trading = true;`, userID).Scan(&cardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FundingSource{Kind: "wallet", UserID: userID}, nil
		}
		return FundingSource{}, err
	}
	return FundingSource{Kind: "card", CardID: cardID, UserID: userID}, nil
}

func GetCardTier(ctx context.Context, q Queryer, cardID int) (string, error) {
	var tier string
	err := q.QueryRow(ctx, `SELECT tier FROM bank_cards WHERE id = $1;`, cardID).Scan(&tier)
	return tier, err
}

func AdjustFundingBalance(ctx context.Context, q Queryer, source FundingSource, delta float64) error {
	var table, whereCol string
	var whereVal interface{}
	if source.Kind == "card" {
		table = "bank_cards"
		whereCol = "id"
		whereVal = source.CardID
	} else {
		table = "wallets"
		whereCol = "user_id"
		whereVal = source.UserID
	}

	if delta >= 0 {
		sqlQuery := "UPDATE " + table + " SET balance = balance + $1 WHERE " + whereCol + " = $2;"
		_, err := q.Exec(ctx, sqlQuery, delta, whereVal)
		return err
	}

	var newBal float64
	sqlQuery := "UPDATE " + table + " SET balance = balance + $1 WHERE " + whereCol + " = $2 AND balance >= $3 RETURNING balance;"
	err := q.QueryRow(ctx, sqlQuery, delta, whereVal, -delta).Scan(&newBal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInsufficientFunds
		}
		return err
	}
	return nil
}

func GetFundingBalance(ctx context.Context, q Queryer, source FundingSource) (float64, error) {
	var bal float64
	if source.Kind == "card" {
		err := q.QueryRow(ctx, `SELECT balance FROM bank_cards WHERE id = $1;`, source.CardID).Scan(&bal)
		return bal, err
	}
	err := q.QueryRow(ctx, `SELECT balance FROM wallets WHERE user_id = $1;`, source.UserID).Scan(&bal)
	return bal, err
}

func ResolveAnyCard(ctx context.Context, q Queryer, userID int) (FundingSource, error) {
	var cardID int
	err := q.QueryRow(ctx, `
		SELECT id FROM bank_cards WHERE user_id = $1
		ORDER BY is_active_for_trading DESC, opened_at DESC
		LIMIT 1;
	`, userID).Scan(&cardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FundingSource{Kind: "wallet", UserID: userID}, nil
		}
		return FundingSource{}, err
	}
	return FundingSource{Kind: "card", CardID: cardID, UserID: userID}, nil
}
