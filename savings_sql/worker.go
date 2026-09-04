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
		t := time.NewTicker(10 * time.Minute)
		for range t.C {
			runAccrualPass(pool)
		}
	}()
	log.Println("savings interest worker started (checks every 10min, accrues hourly)")
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
		if err := rows.Scan(&r.userID, &r.balance, &r.lastAccruedAt); err == nil {
			accounts = append(accounts, r)
		}
	}
	rows.Close()

	for _, acc := range accounts {
		hoursElapsed := time.Since(acc.lastAccruedAt).Hours()
		if hoursElapsed < 1 {
			continue
		}

		interest := acc.balance * (AnnualInterestRatePercent / 100) * hoursElapsed / (365 * 24)
		if interest <= 0 {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			continue
		}

		_, err = tx.Exec(ctx, `UPDATE savings_accounts SET balance = balance + $1, last_accrued_at = now() WHERE user_id = $2;`, interest, acc.userID)
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
		} else {
			log.Println("savings worker: accrued", interest, "for user", acc.userID)
		}
	}
}
