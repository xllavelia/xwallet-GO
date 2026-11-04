package battlepass_sql

import (
	"context"
	"log"
	"time"

	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

// applyBoostAndAdd awards base XP with the active voucher XP boost only.
func applyBoostAndAdd(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	baseXP int,
) int {
	if baseXP <= 0 {
		return 0
	}

	boostPercent, err := user_vouchers_sql.GetActiveXpBoostPercent(ctx, pool, userID)
	if err != nil {
		boostPercent = 0
	}

	finalXP := int(float64(baseXP) * (1 + boostPercent/100))

	if finalXP <= 0 {
		return 0
	}

	_, err = pool.Exec(
		ctx,
		`UPDATE battlepass_progress
		 SET xp = xp + $1
		 WHERE user_id = $2;`,
		finalXP,
		userID,
	)
	if err != nil {
		log.Println("battlepass xp award failed:", err)
		return 0
	}

	return finalXP
}

func tradeBaseXP(pnl float64) int {
	if pnl > 0 {
		switch {
		case pnl >= 100:
			return 300
		case pnl >= 10:
			return 100
		}
	} else if pnl < 0 {
		loss := -pnl
		switch {
		case loss >= 10:
			return 300
		case loss >= 1:
			return 100
		}
	}
	return 0
}

// AwardTradeXP preserves profit/loss XP and active voucher boosts, independent
// of whether the trade was funded by an ordinary account or the wallet.
func AwardTradeXP(ctx context.Context, pool *pgxpool.Pool, userID int, pnl float64) int {
	baseXP := tradeBaseXP(pnl)
	if baseXP <= 0 {
		return 0
	}
	if err := ensureRow(ctx, pool, userID); err != nil {
		log.Println("battlepass ensure row failed:", err)
		return 0
	}
	return applyBoostAndAdd(ctx, pool, userID, baseXP)
}

// AwardTransferXP начисляет XP за перевод.
// 20$ — минимальная сумма для получения 50 XP.
// 200$ — минимальная сумма для получения 300 XP.
// Повторно получить XP можно через 24 часа.
func AwardTransferXP(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	amount float64,
) int {
	return awardDailyCapped(
		ctx,
		pool,
		userID,
		amount,
		20,
		50,
		200,
		300,
		"last_transfer_xp_at",
	)
}

// AwardCardBuyXP начисляет XP за покупку карты.
// 20$ — минимальная сумма для получения 200 XP.
// Повторно получить XP можно через 24 часа.
func AwardCardBuyXP(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	usdAmount float64,
) int {
	return awardDailyCapped(
		ctx,
		pool,
		userID,
		usdAmount,
		20,
		200,
		0,
		0,
		"last_card_buy_xp_at",
	)
}

// AwardCardSellXP начисляет XP за продажу карты.
// 20$ — минимальная сумма для получения 20 XP.
// Повторно получить XP можно через 24 часа.
func AwardCardSellXP(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	usdAmount float64,
) int {
	return awardDailyCapped(
		ctx,
		pool,
		userID,
		usdAmount,
		20,
		20,
		0,
		0,
		"last_card_sell_xp_at",
	)
}

// awardDailyCapped проверяет дневной лимит для конкретного действия,
// начисляет XP и записывает время последнего получения XP.
//
// column используется только с заранее заданными значениями,
// передаваемыми из функций выше.
func awardDailyCapped(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	amount float64,
	tier1Min float64,
	tier1XP int,
	tier2Min float64,
	tier2XP int,
	column string,
) int {
	if amount <= 0 {
		return 0
	}

	if err := ensureRow(ctx, pool, userID); err != nil {
		log.Println("battlepass ensure row failed:", err)
		return 0
	}

	var lastAt *time.Time

	query := `SELECT ` + column + `
		FROM battlepass_progress
		WHERE user_id = $1;`

	if err := pool.QueryRow(
		ctx,
		query,
		userID,
	).Scan(&lastAt); err != nil {
		log.Println("battlepass last xp time query failed:", err)
		return 0
	}

	// Не позволяем получать XP повторно раньше чем через 24 часа.
	if lastAt != nil && time.Since(*lastAt) < 24*time.Hour {
		return 0
	}

	baseXP := 0

	if tier2Min > 0 && amount >= tier2Min {
		baseXP = tier2XP
	} else if tier1Min > 0 && amount >= tier1Min {
		baseXP = tier1XP
	}

	if baseXP <= 0 {
		return 0
	}

	updateQuery := `UPDATE battlepass_progress
		SET ` + column + ` = NOW()
		WHERE user_id = $1;`

	if _, err := pool.Exec(
		ctx,
		updateQuery,
		userID,
	); err != nil {
		log.Println("battlepass xp timestamp update failed:", err)
		return 0
	}

	return applyBoostAndAdd(ctx, pool, userID, baseXP)
}

// DevAddXP позволяет вручную добавить XP пользователю.
// Используется для разработки/тестирования Battle Pass.
func DevAddXP(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	amount int,
) int {
	if amount <= 0 {
		return 0
	}

	if err := ensureRow(ctx, pool, userID); err != nil {
		log.Println("battlepass ensure row failed:", err)
		return 0
	}

	return applyBoostAndAdd(ctx, pool, userID, amount)
}
