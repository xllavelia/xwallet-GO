package referral_sql

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreditReferrerIfAny(ctx context.Context, pool *pgxpool.Pool, referredUserID int, feesCharged float64, tradeVolume float64) {
	var referrerUserID int
	err := pool.QueryRow(ctx, `SELECT referrer_user_id FROM referral_links WHERE referred_user_id = $1;`, referredUserID).Scan(&referrerUserID)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Println("referral lookup error:", err)
		}
		return
	}

	var refXp int
	err = pool.QueryRow(ctx, `SELECT ref_xp FROM referrals WHERE user_id = $1;`, referrerUserID).Scan(&refXp)
	if err != nil {
		return
	}

	rate := RateForLevel(LevelFromXP(refXp))
	commission := feesCharged * (rate / 100)
	if commission <= 0 {
		return
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;`, commission, referrerUserID); err != nil {
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE referrals SET total_earned = total_earned + $1, total_referred_volume = total_referred_volume + $2 WHERE user_id = $3;`, commission, tradeVolume, referrerUserID); err != nil {
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Println("referral commission commit failed:", err)
	}
}
