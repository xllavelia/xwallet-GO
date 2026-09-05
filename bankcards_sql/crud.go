package bankcards_sql

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTierNotFound = errors.New("tier not found")
var ErrMaxCardsReached = errors.New("maximum number of cards reached")
var ErrTierAlreadyOwned = errors.New("you already own a card of this tier")
var ErrInsufficientWalletBalance = errors.New("insufficient wallet balance")
var ErrCardNotFound = errors.New("card not found")

func generateCardNumber() string {
	digits := "0123456789"
	b := make([]byte, 16)
	rand.Read(b)
	out := make([]byte, 16)
	for i, v := range b {
		out[i] = digits[int(v)%10]
	}
	return string(out)
}

func ListCards(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Card, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, tier, card_number, balance, is_active_for_trading, last_lavx_grant_at, opened_at
		FROM bank_cards WHERE user_id = $1 ORDER BY opened_at ASC;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.UserID, &c.Tier, &c.CardNumber, &c.Balance, &c.IsActiveForTrading, &c.LastLavxGrantAt, &c.OpenedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func OpenCard(ctx context.Context, pool *pgxpool.Pool, userID int, tier string) (Card, error) {
	cfg, ok := Tiers[tier]
	if !ok {
		return Card{}, ErrTierNotFound
	}

	var alreadyOwnsTier bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bank_cards WHERE user_id = $1 AND tier = $2);`, userID, tier).Scan(&alreadyOwnsTier); err != nil {
		return Card{}, err
	}
	if alreadyOwnsTier {
		return Card{}, ErrTierAlreadyOwned
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback(ctx)

	if cfg.OpenPriceUsd > 0 {
		var newBal float64
		err := tx.QueryRow(ctx, `
			UPDATE wallets SET balance = balance - $1, updated_at = now()
			WHERE user_id = $2 AND balance >= $1
			RETURNING balance;
		`, cfg.OpenPriceUsd, userID).Scan(&newBal)
		if err != nil {
			return Card{}, ErrInsufficientWalletBalance
		}
	}

	var card Card
	var cardNumber string
	for attempt := 0; attempt < 20; attempt++ {
		cardNumber = generateCardNumber()
		var exists bool
		tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bank_cards WHERE card_number = $1);`, cardNumber).Scan(&exists)
		if !exists {
			break
		}
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO bank_cards (user_id, tier, card_number)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, tier, card_number, balance, is_active_for_trading, opened_at;
	`, userID, tier, cardNumber).Scan(&card.ID, &card.UserID, &card.Tier, &card.CardNumber, &card.Balance, &card.IsActiveForTrading, &card.OpenedAt)
	if err != nil {
		return Card{}, err
	}

	return card, tx.Commit(ctx)
}

func TopUpCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int, amount float64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var owner int
	if err := tx.QueryRow(ctx, `SELECT user_id FROM bank_cards WHERE id = $1;`, cardID).Scan(&owner); err != nil {
		return ErrCardNotFound
	}
	if owner != userID {
		return ErrCardNotFound
	}

	walletSource := FundingSource{Kind: "wallet", UserID: userID}
	if err := AdjustFundingBalance(ctx, tx, walletSource, -amount); err != nil {
		return err
	}
	cardSource := FundingSource{Kind: "card", CardID: cardID}
	if err := AdjustFundingBalance(ctx, tx, cardSource, amount); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func SelectActiveCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var owner int
	if err := tx.QueryRow(ctx, `SELECT user_id FROM bank_cards WHERE id = $1;`, cardID).Scan(&owner); err != nil {
		return ErrCardNotFound
	}
	if owner != userID {
		return ErrCardNotFound
	}

	if _, err := tx.Exec(ctx, `UPDATE bank_cards SET is_active_for_trading = false WHERE user_id = $1;`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bank_cards SET is_active_for_trading = true WHERE id = $1;`, cardID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func CloseCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int) error {
	tag, err := pool.Exec(ctx, `DELETE FROM bank_cards WHERE id = $1 AND user_id = $2;`, cardID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCardNotFound
	}
	return nil
}

func ResolveByCardNumber(ctx context.Context, pool *pgxpool.Pool, number string) (int, string, error) {
	var userID int
	var username string
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.username FROM bank_cards bc
		JOIN users u ON u.id = bc.user_id
		WHERE bc.card_number = $1;
	`, number).Scan(&userID, &username)
	return userID, username, err
}

var _ = time.Now
