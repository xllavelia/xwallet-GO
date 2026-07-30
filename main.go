package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/card_sql"
	"xwallet-server/db_connection"
	"xwallet-server/positions_http"
	"xwallet-server/positions_sql"
	"xwallet-server/prime_sql"
	"xwallet-server/promo_sql"
	"xwallet-server/referral_sql"
	"xwallet-server/transfer_sql"
	"xwallet-server/users_sql"
	"xwallet-server/voucher_sql"
	"xwallet-server/wallet_http"
	"xwallet-server/wallet_sql"
)

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is not set")
	}
	if os.Getenv("ALLOWED_ORIGIN") == "" {
		log.Println("WARNING: ALLOWED_ORIGIN is not set — CORS will block your frontend")
	}

	ctx := context.Background()

	pool, err := db_connection.CreateConnection(ctx)
	if err != nil {
		log.Fatal("could not connect to database: ", err)
	}
	defer pool.Close()

	if err := users_sql.CreateUsersTable(ctx, pool); err != nil {
		log.Fatal("users table: ", err)
	}
	if err := wallet_sql.CreateWalletsTable(ctx, pool); err != nil {
		log.Fatal("wallets table: ", err)
	}
	if err := prime_sql.CreatePrimeSubscriptionsTable(ctx, pool); err != nil {
		log.Fatal("prime_subscriptions table: ", err)
	}
	if err := referral_sql.CreateReferralsTable(ctx, pool); err != nil {
		log.Fatal("referrals table: ", err)
	}
	if err := referral_sql.CreateReferralLinksTable(ctx, pool); err != nil {
		log.Fatal("referral_links table: ", err)
	}
	if err := referral_sql.CreateCommissionRatesTable(ctx, pool); err != nil {
		log.Fatal("referral_commission_rates table: ", err)
	}
	if err := referral_sql.SeedCommissionRates(ctx, pool); err != nil {
		log.Fatal("seeding commission rates: ", err)
	}
	if err := voucher_sql.CreateVouchersTable(ctx, pool); err != nil {
		log.Fatal("vouchers table: ", err)
	}
	if err := card_sql.CreateCryptoCardsTable(ctx, pool); err != nil {
		log.Fatal("crypto_cards table: ", err)
	}
	if err := positions_sql.CreatePositionsTable(ctx, pool); err != nil {
		log.Fatal("positions table: ", err)
	}
	if err := transfer_sql.CreateTransfersTable(ctx, pool); err != nil {
		log.Fatal("transfers table: ", err)
	}
	if err := battlepass_sql.CreateBattlepassProgressTable(ctx, pool); err != nil {
		log.Fatal("battlepass_progress table: ", err)
	}
	if err := promo_sql.CreatePromoCodesTable(ctx, pool); err != nil {
		log.Fatal("promo_codes table: ", err)
	}
	if err := promo_sql.SeedPromoCodes(ctx, pool); err != nil {
		log.Fatal("seeding promo codes: ", err)
	}

	log.Println("all tables ready")

	adminExists, err := users_sql.PlayerIDExists(ctx, pool, "000001")
	if err != nil {
		log.Fatal(err)
	}
	if !adminExists {
		err := users_sql.InsertUser(ctx, pool, "000001", "xlavelia",
			"$2b$10$XGfUWvKFBaSEYmn/4ZcTW.GJVLegoAZmBV3FaBL92cDz7vAvjFQA.", true)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("admin account seeded")
	}

	http.HandleFunc("/auth/register", auth_http.WithCORS(auth_http.RegisterHandler(pool)))
	http.HandleFunc("/auth/login", auth_http.WithCORS(auth_http.LoginHandler(pool)))
	http.HandleFunc("/auth/generate-id", auth_http.WithCORS(auth_http.GenerateIDHandler(pool)))
	http.HandleFunc("/auth/me", auth_http.WithCORS(auth_http.RequireAuth(auth_http.MeHandler)))
	http.HandleFunc("/positions/open", auth_http.WithCORS(auth_http.RequireAuth(positions_http.OpenPositionHandler(pool))))
	http.HandleFunc("/positions/close", auth_http.WithCORS(auth_http.RequireAuth(positions_http.ClosePositionHandler(pool))))
	http.HandleFunc("/positions/open-list", auth_http.WithCORS(auth_http.RequireAuth(positions_http.ListOpenPositionsHandler(pool))))
	http.HandleFunc("/wallet", auth_http.WithCORS(auth_http.RequireAuth(wallet_http.GetWalletHandler(pool))))
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	positions_http.StartLiquidationWorker(pool)
	log.Println("Auth server running on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
