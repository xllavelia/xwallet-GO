package bankcards_sql

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartLavxWorker(pool *pgxpool.Pool) {
	go func() {
		runLavxPass(pool)
		t := time.NewTicker(1 * time.Hour)
		for range t.C {
			runLavxPass(pool)
		}
	}()
	log.Println("bank card LAVX worker started (hourly)")
}

func runLavxPass(pool *pgxpool.Pool) {
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT id, user_id, tier, last_lavx_grant_at, opened_at FROM bank_cards WHERE tier != 'standard';`)
	if err != nil {
		return
	}
	type row struct {
		id, userID int
		tier       string
		lastAt     *time.Time
		openedAt   time.Time
	}
	var items []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.userID, &r.tier, &r.lastAt, &r.openedAt); err == nil {
			items = append(items, r)
		}
	}
	rows.Close()

	for _, it := range items {
		cfg, ok := Tiers[it.tier]
		if !ok || cfg.LavxPerMonth <= 0 {
			continue
		}
		base := it.openedAt
		if it.lastAt != nil {
			base = *it.lastAt
		}
		if time.Since(base) < 30*24*time.Hour {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE wallets SET lavx_balance = lavx_balance + $1 WHERE user_id = $2;`, cfg.LavxPerMonth, it.userID); err != nil {
			tx.Rollback(ctx)
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE bank_cards SET last_lavx_grant_at = now() WHERE id = $1;`, it.id); err != nil {
			tx.Rollback(ctx)
			continue
		}
		tx.Commit(ctx)
	}
}
