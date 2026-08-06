package positions_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SetXpAwarded(ctx context.Context, pool *pgxpool.Pool, id int, xp int) {
	pool.Exec(ctx, `UPDATE positions SET xp_awarded = $1 WHERE id = $2;`, xp, id)
}
