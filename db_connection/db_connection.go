package db_connection

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateConnection(ctx context.Context) (*pgxpool.Pool, error) {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://postgres:1224@localhost:5432/postgres"
	}
	return pgxpool.New(ctx, connString)
}
