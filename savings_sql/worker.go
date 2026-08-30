package savings_sql

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartInterestWorker(pool *pgxpool.Pool) {
	go func() {
		runAccrualPass(pool)
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			runAccrualPass(pool)
		}
	}()
	log.Println("savings interest worker started (hourly)")
}

type accrualRow struct {
	userID        int
	balance       float64
	lastAccruedAt time.Time
}

func runAccrualPass(pool *pgxpool.Pool) {
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT user_id, balance, last_accrued_at FROM savings_accounts WHERE balance > 0;`)
	if err != nil {
		log.Println("savings worker: could not load accounts:", err)
		return
	}

	var accounts []accrualRow
	for rows.Next() {
		var r accrualRow
		if err := rows.Scan(&r.userID, &r.balance, &r.lastAccruedAt); err != nil {
			continue
		}
		accounts = append(accounts, r)
	}
	rows.Close()

	for _, acc := range accounts {
		days := int(time.Since(acc.lastAccruedAt).Hours() / 24)
		if days < 1 {
			continue
		}

		interest := acc.balance * (AnnualInterestRatePercent / 100) * float64(days) / 365
		if interest <= 0 {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			continue
		}

		_, err = tx.Exec(ctx, `
			UPDATE savings_accounts
			SET balance = balance + $1, last_accrued_at = last_accrued_at + ($2 || ' days')::interval
			WHERE user_id = $3;
		`, interest, days, acc.userID)
		if err != nil {
			tx.Rollback(ctx)
			continue
		}

		_, err = tx.Exec(ctx, `INSERT INTO savings_history (user_id, entry_type, amount) VALUES ($1, 'interest', $2);`, acc.userID, interest)
		if err != nil {
			tx.Rollback(ctx)
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			log.Println("savings worker: commit failed for user", acc.userID, err)
		}
	}
}
