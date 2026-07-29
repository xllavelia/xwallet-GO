package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"xwallet-server/auth_http"
	"xwallet-server/db_connection"
	"xwallet-server/users_sql"
)

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is not set — set it in Render → your web service → Environment")
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
		log.Fatal("could not create users table: ", err)
	}
	log.Println("users table ready")

	adminExists, err := users_sql.PlayerIDExists(ctx, pool, "000001")
	if err != nil {
		log.Fatal("could not check for admin user: ", err)
	}
	if !adminExists {
		err := users_sql.InsertUser(ctx, pool, "000001", "xlavelia",
			"$2b$10$XGfUWvKFBaSEYmn/4ZcTW.GJVLegoAZmBV3FaBL92cDz7vAvjFQA.", true)
		if err != nil {
			log.Fatal("could not seed admin user: ", err)
		}
		log.Println("admin account seeded")
	} else {
		log.Println("admin account already exists")
	}

	http.HandleFunc("/auth/register", auth_http.WithCORS(auth_http.RegisterHandler(pool)))
	http.HandleFunc("/auth/login", auth_http.WithCORS(auth_http.LoginHandler(pool)))
	http.HandleFunc("/auth/generate-id", auth_http.WithCORS(auth_http.GenerateIDHandler(pool)))
	http.HandleFunc("/auth/me", auth_http.WithCORS(auth_http.RequireAuth(auth_http.MeHandler)))
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Auth server running on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
