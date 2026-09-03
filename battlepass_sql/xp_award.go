package battlepass_sql

import (
	"context"
	"log"
	"time"

	"xwallet-server/bankcards_sql"
	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

// applyBoostAndAdd начисляет XP с учетом активного XP-бустера
// и дополнительного бонуса, например от банковской карты.
func applyBoostAndAdd(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	baseXP int,
	extraPercent float64,
) int {
	if baseXP <= 0 {
		return 0
	}

	boostPercent, err := user_vouchers_sql.GetActiveXpBoostPercent(ctx, pool, userID)
	if err != nil {
		boostPercent = 0
	}

	totalPercent := boostPercent + extraPercent
	finalXP := int(float64(baseXP) * (1 + totalPercent/100))

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

// AwardTradeXP начисляет XP за совершенную сделку.
// XP зависит от прибыли/убытка и может дополнительно увеличиваться
// за счет бонуса банковской карты и активного XP-бустера.
func AwardTradeXP(
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int,
	pnl float64,
	source bankcards_sql.FundingSource,
) int {
	var baseXP int

	if pnl > 0 {
		switch {
		case pnl >= 100:
			baseXP = 300
		case pnl >= 10:
			baseXP = 100
		}
	} else if pnl < 0 {
		loss := -pnl

		switch {
		case loss >= 10:
			baseXP = 300
		case loss >= 1:
			baseXP = 100
		}
	}

	if baseXP <= 0 {
		return 0
	}

	if err := ensureRow(ctx, pool, userID); err != nil {
		log.Println("battlepass ensure row failed:", err)
		return 0
	}

	cardBonus := 0.0

	if source.Kind == "card" {
		tier, err := bankcards_sql.GetCardTier(
			ctx,
			pool,
			source.CardID,
		)

		if err == nil {
			if cfg, ok := bankcards_sql.Tiers[tier]; ok {
				cardBonus = cfg.XpBonusPercent
			}
		}
	}

	return applyBoostAndAdd(
		ctx,
		pool,
		userID,
		baseXP,
		cardBonus,
	)
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

	return applyBoostAndAdd(
		ctx,
		pool,
		userID,
		baseXP,
		0,
	)
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

	return applyBoostAndAdd(
		ctx,
		pool,
		userID,
		amount,
		0,
	)
}
