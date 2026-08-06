package battlepass_sql

import (
	"context"
	"log"
	"time"

	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func applyBoostAndAdd(ctx context.Context, pool *pgxpool.Pool, userID int, baseXp int) int {
	if baseXp <= 0 {
		return 0
	}
	boostPercent, _ := user_vouchers_sql.GetActiveXpBoostPercent(ctx, pool, userID)
	finalXp := int(float64(baseXp) * (1 + boostPercent/100))

	_, err := pool.Exec(ctx, `UPDATE battlepass_progress SET xp = xp + $1 WHERE user_id = $2;`, finalXp, userID)
	if err != nil {
		log.Println("battlepass xp award failed:", err)
		return 0
	}
	return finalXp
}

func AwardTradeXP(ctx context.Context, pool *pgxpool.Pool, userID int, pnl float64) int {
	var baseXp int
	if pnl > 0 {
		if pnl >= 100 {
			baseXp = 300
		} else if pnl >= 10 {
			baseXp = 100
		}
	} else if pnl < 0 {
		loss := -pnl
		if loss >= 10 {
			baseXp = 300
		} else if loss >= 1 {
			baseXp = 100
		}
	}
	if baseXp == 0 {
		return 0
	}
	if err := ensureRow(ctx, pool, userID); err != nil {
		return 0
	}
	return applyBoostAndAdd(ctx, pool, userID, baseXp)
}

func AwardTransferXP(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64) int {
	return awardDailyCapped(ctx, pool, userID, amount, 20, 200, 50, 300, "last_transfer_xp_at")
}

func AwardCardBuyXP(ctx context.Context, pool *pgxpool.Pool, userID int, usdAmount float64) int {
	return awardDailyCapped(ctx, pool, userID, usdAmount, 20, 200, 0, 0, "last_card_buy_xp_at")
}

func AwardCardSellXP(ctx context.Context, pool *pgxpool.Pool, userID int, usdAmount float64) int {
	return awardDailyCapped(ctx, pool, userID, usdAmount, 20, 20, 0, 0, "last_card_sell_xp_at")
}

func awardDailyCapped(ctx context.Context, pool *pgxpool.Pool, userID int, amount float64, tier1Min float64, tier1Xp int, tier2Min float64, tier2Xp int, column string) int {
	if err := ensureRow(ctx, pool, userID); err != nil {
		return 0
	}

	var lastAt *time.Time
	sqlQuery := `SELECT ` + column + ` FROM battlepass_progress WHERE user_id = $1;`
	if err := pool.QueryRow(ctx, sqlQuery, userID).Scan(&lastAt); err != nil {
		return 0
	}
	if lastAt != nil && time.Since(*lastAt) < 24*time.Hour {
		return 0
	}

	baseXp := 0
	if tier2Min > 0 && amount >= tier2Min {
		baseXp = tier2Xp
	} else if amount >= tier1Min {
		baseXp = tier1Xp
	}
	if baseXp == 0 {
		return 0
	}

	updateQuery := `UPDATE battlepass_progress SET ` + column + ` = now() WHERE user_id = $1;`
	if _, err := pool.Exec(ctx, updateQuery, userID); err != nil {
		return 0
	}

	return applyBoostAndAdd(ctx, pool, userID, baseXp)
}

func DevAddXP(ctx context.Context, pool *pgxpool.Pool, userID int, amount int) int {
	if err := ensureRow(ctx, pool, userID); err != nil {
		return 0
	}
	return applyBoostAndAdd(ctx, pool, userID, amount)
}
