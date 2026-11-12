package bankcards_sql

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMaxCardsReached = errors.New("maximum of 5 accounts reached")
var ErrInvalidName = errors.New("name must contain 1 to 32 characters and no control characters")
var ErrInvalidAmount = errors.New("amount must be finite, positive, and in whole cents")
var ErrCardNotFound = errors.New("card not found")
var ErrCardFunded = errors.New("cannot close an account with a nonzero balance")
var ErrCardHasOpenPositions = errors.New("cannot close an account funding open positions")

// ValidateOpenCard проверяет имя счёта; оформление проверяется отдельно через ValidateDesignChoice.
func ValidateOpenCard(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidName
		}
	}
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 32 {
		return "", ErrInvalidName
	}
	return name, nil
}

func ValidateCardAmount(amount float64) error {
	// Допуск учитывает только погрешность float64, но не доли цента.
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount >= 1e12 {
		return ErrInvalidAmount
	}
	cents := amount * 100
	if math.Round(cents) < 1 || math.Abs(cents-math.Round(cents)) > 1e-6 {
		return ErrInvalidAmount
	}
	return nil
}

func generateCardNumber() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i, v := range b {
		b[i] = '0' + v%10
	}
	return string(b), nil
}

func lockUser(ctx context.Context, q Queryer, userID int) error {
	var id int
	return q.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE;`, userID).Scan(&id)
}

func ListCards(ctx context.Context, pool *pgxpool.Pool, userID int) ([]Card, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, tier, name, design_id, design_key, custom, card_number, balance, is_active_for_trading, last_lavx_grant_at, opened_at
		FROM bank_cards WHERE user_id = $1 ORDER BY opened_at ASC, id ASC;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Card{}
	for rows.Next() {
		var c Card
		var customRaw []byte
		if err := rows.Scan(&c.ID, &c.UserID, &c.Tier, &c.Name, &c.DesignID, &c.DesignKey, &customRaw, &c.CardNumber, &c.Balance, &c.IsActiveForTrading, &c.LastLavxGrantAt, &c.OpenedAt); err != nil {
			return nil, err
		}
		if c.Custom, err = scanCustom(customRaw); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func openCardTx(ctx context.Context, tx pgx.Tx, userID int, name string, choice DesignChoice) (Card, error) {
	// Блокировка пользователя работает и до открытия первого счёта.
	if err := lockUser(ctx, tx, userID); err != nil {
		return Card{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM bank_cards WHERE user_id = $1;`, userID).Scan(&count); err != nil {
		return Card{}, err
	}
	if count >= MaxCardsPerUser {
		return Card{}, ErrMaxCardsReached
	}

	for attempt := 0; attempt < 20; attempt++ {
		cardNumber, err := generateCardNumber()
		if err != nil {
			return Card{}, err
		}
		var card Card
		var customRaw []byte
		err = tx.QueryRow(ctx, `
			INSERT INTO bank_cards (user_id, tier, name, design_id, design_key, custom, card_number, balance, is_active_for_trading)
			VALUES ($1, 'standard', $2, $3, $4, $5, $6, 0, false)
			ON CONFLICT (card_number) DO NOTHING
			RETURNING id, user_id, tier, name, design_id, design_key, custom, card_number, balance, is_active_for_trading, opened_at;
		`, userID, name, choice.DesignID, nullIfEmpty(choice.DesignKey), customJSON(choice.Custom), cardNumber).Scan(
			&card.ID, &card.UserID, &card.Tier, &card.Name, &card.DesignID, &card.DesignKey, &customRaw, &card.CardNumber,
			&card.Balance, &card.IsActiveForTrading, &card.OpenedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return Card{}, err
		}
		if card.Custom, err = scanCustom(customRaw); err != nil {
			return Card{}, err
		}
		return card, nil
	}
	return Card{}, errors.New("could not generate a unique card number")
}

func TopUpCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int, amount float64) error {
	if err := ValidateCardAmount(amount); err != nil {
		return err
	}
	amount = math.Round(amount*100) / 100
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := moveCardFundsTx(ctx, tx, userID, cardID, amount, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func moveCardFundsTx(ctx context.Context, tx pgx.Tx, userID, cardID int, amount float64, topUp bool) error {
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if err := lockOwnedCard(ctx, tx, userID, cardID); err != nil {
		return err
	}
	from := FundingSource{Kind: "wallet", UserID: userID}
	to := FundingSource{Kind: "card", CardID: cardID}
	if !topUp {
		from, to = to, from
	}
	if err := AdjustFundingBalance(ctx, tx, from, -amount); err != nil {
		return err
	}
	return AdjustFundingBalance(ctx, tx, to, amount)
}

func lockOwnedCard(ctx context.Context, q Queryer, userID, cardID int) error {
	var id int
	err := q.QueryRow(ctx, `SELECT id FROM bank_cards WHERE id = $1 AND user_id = $2 FOR UPDATE;`, cardID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCardNotFound
	}
	return err
}

func SelectActiveCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := selectActiveCardTx(ctx, tx, userID, cardID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func selectActiveCardTx(ctx context.Context, tx pgx.Tx, userID, cardID int) error {
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if cardID != 0 {
		if err := lockOwnedCard(ctx, tx, userID, cardID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE bank_cards SET is_active_for_trading = false WHERE user_id = $1;`, userID); err != nil {
		return err
	}
	if cardID != 0 {
		if _, err := tx.Exec(ctx, `UPDATE bank_cards SET is_active_for_trading = true WHERE id = $1;`, cardID); err != nil {
			return err
		}
	}
	return nil
}

func CloseCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := closeCardTx(ctx, tx, userID, cardID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func closeCardTx(ctx context.Context, tx pgx.Tx, userID, cardID int) error {
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var balance float64
	err := tx.QueryRow(ctx, `SELECT balance FROM bank_cards WHERE id = $1 AND user_id = $2 FOR UPDATE;`, cardID, userID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCardNotFound
	}
	if err != nil {
		return err
	}
	if balance != 0 {
		return ErrCardFunded
	}
	// Блокировка счёта удерживает параллельные вставки позиций через внешний ключ.
	var hasOpenPositions bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM positions WHERE funding_card_id = $1 AND status = 'open');`, cardID).Scan(&hasOpenPositions); err != nil {
		return err
	}
	if hasOpenPositions {
		return ErrCardHasOpenPositions
	}
	tag, err := tx.Exec(ctx, `DELETE FROM bank_cards WHERE id = $1 AND user_id = $2;`, cardID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
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

func WithdrawFromCard(ctx context.Context, pool *pgxpool.Pool, userID int, cardID int, amount float64) error {
	if err := ValidateCardAmount(amount); err != nil {
		return err
	}
	amount = math.Round(amount*100) / 100
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := moveCardFundsTx(ctx, tx, userID, cardID, amount, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
