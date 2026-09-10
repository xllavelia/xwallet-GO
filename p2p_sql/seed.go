package p2p_sql

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SeedOfficialListings(ctx context.Context, pool *pgxpool.Pool) {
	var adminID int
	err := pool.QueryRow(ctx, `SELECT id FROM users WHERE player_id = '000001';`).Scan(&adminID)
	if err != nil {
		return
	}

	var alreadySeeded bool
	pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM p2p_listings WHERE user_id=$1 AND base_asset='LAVX');`, adminID).Scan(&alreadySeeded)
	if alreadySeeded {
		return
	}

	var isMerchant bool
	pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM p2p_merchants WHERE user_id=$1);`, adminID).Scan(&isMerchant)
	if !isMerchant {
		pool.Exec(ctx, `INSERT INTO p2p_merchants (user_id, is_verified, deposit_amount) VALUES ($1, true, 0);`, adminID)
	}

	pool.Exec(ctx, `UPDATE wallets SET balance = balance + 5000, lavx_balance = lavx_balance + 500 WHERE user_id = $1;`, adminID)

	if _, err := CreateListing(ctx, pool, adminID, "sell", "lavx", "LAVX", 10.0, 10, 2000, 500, "Official xwallet LAVX desk. Instant settlement."); err != nil {
		log.Println("p2p seed: could not create official sell listing:", err)
	}
	if _, err := CreateListing(ctx, pool, adminID, "buy", "lavx", "LAVX", 10.0, 10, 2000, 500, "Official xwallet LAVX desk. Instant settlement."); err != nil {
		log.Println("p2p seed: could not create official buy listing:", err)
	}
}
