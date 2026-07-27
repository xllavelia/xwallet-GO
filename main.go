package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"xwallet-server/auth_http"
	"xwallet-server/db_connection"
	"xwallet-server/users_sql"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if os.Getenv("JWT_SECRET") == "" {
		log.Println("WARNING: JWT_SECRET is not set — using an insecure default. Set it before deploying.")
	}
	if os.Getenv("ALLOWED_ORIGIN") == "" {
		log.Println("WARNING: ALLOWED_ORIGIN is not set — defaulting to http://localhost:5173.")
	}

	ctx := context.Background()

	pool, err := db_connection.CreateConnection(ctx)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	if err := users_sql.CreateUsersTable(ctx, pool); err != nil {
		panic(err)
	}

	adminExists, err := users_sql.PlayerIDExists(ctx, pool, "000001")
	if err != nil {
		panic(err)
	}
	if !adminExists {
		err := users_sql.InsertUser(ctx, pool, "000001", "xlavelia",
			"$2b$10$XGfUWvKFBaSEYmn/4ZcTW.GJVLegoAZmBV3FaBL92cDz7vAvjFQA.", true)
		if err != nil {
			panic(err)
		}
		log.Println("admin account seeded")
	}

	http.HandleFunc("/auth/register", auth_http.WithCORS(auth_http.RegisterHandler(pool)))
	http.HandleFunc("/auth/login", auth_http.WithCORS(auth_http.LoginHandler(pool)))
	http.HandleFunc("/auth/generate-id", auth_http.WithCORS(auth_http.GenerateIDHandler(pool)))
	http.HandleFunc("/auth/me", auth_http.WithCORS(auth_http.RequireAuth(auth_http.MeHandler)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Auth server running on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
