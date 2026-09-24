package users_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Продовая схема дрейфует: таблица может отсутствовать (42P01)
// или не иметь ожидаемой колонки (42703). Такие шаги пропускаем,
// иначе удаление одного юзера снова падает в 500.
func isSkippableSchemaError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01" || pgErr.Code == "42703"
	}
	return false
}

// Удаляются первыми: ссылаются на users через колонки с другими именами.
// p2p_deals обязан идти до p2p_listings.
var childQueries = []string{
	`DELETE FROM p2p_deals WHERE poster_user_id = $1 OR taker_user_id = $1;`,
	`DELETE FROM referral_links WHERE referrer_user_id = $1 OR referred_user_id = $1;`,
	`DELETE FROM transfers WHERE sender_id = $1 OR recipient_id = $1;`,
	`DELETE FROM contacts WHERE user_id = $1 OR contact_user_id = $1;`,
}

// Порядок важен: история раньше владельцев, сделки раньше листингов.
var childTables = []string{
	"p2p_listings",
	"p2p_merchants",
	"tickets",
	"ticket_packs",
	"ticket_user_stats",
	"flip_bets",
	"pixel_boards",
	"pixel_user_stats",
	"rocket_bets",
	"commodity_history",
	"commodity_holdings",
	"stock_history",
	"stock_holdings",
	"voucher_claims_log",
	"user_vouchers",
	"vouchers",
	"card_history",
	"bank_cards",
	"crypto_cards",
	"savings_history",
	"savings_accounts",
	"promo_code_redemptions",
	"user_statuses",
	"battlepass_progress",
	"prime_subscriptions",
	"positions",
	"referrals",
	"wallets",
}

func DeleteUserByPlayerID(ctx context.Context, pool *pgxpool.Pool, playerID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var internalID int
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE player_id = $1;`, playerID).Scan(&internalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	for _, q := range childQueries {
		if _, err := tx.Exec(ctx, q, internalID); err != nil && !isSkippableSchemaError(err) {
			return err
		}
	}

	for _, table := range childTables {
		q := `DELETE FROM ` + table + ` WHERE user_id = $1;`
		if _, err := tx.Exec(ctx, q, internalID); err != nil && !isSkippableSchemaError(err) {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1;`, internalID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
