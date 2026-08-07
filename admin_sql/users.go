package admin_sql

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRow struct {
	PlayerID    string
	Username    string
	IsAdmin     bool
	CreatedAt   time.Time
	Balance     float64
	LavxBalance float64
	PrimeTier   *string
}

func ListUsers(ctx context.Context, pool *pgxpool.Pool, search string, limit int, offset int) ([]UserRow, error) {
	sqlQuery := `
	SELECT u.player_id, u.username, u.is_admin, u.created_at,
	       COALESCE(w.balance,0), COALESCE(w.lavx_balance,0), ps.tier
	FROM users u
	LEFT JOIN wallets w ON w.user_id = u.id
	LEFT JOIN prime_subscriptions ps ON ps.user_id = u.id AND ps.tier IS NOT NULL AND ps.expires_at > now()
	WHERE u.username ILIKE '%' || $1 || '%' OR u.player_id ILIKE '%' || $1 || '%'
	ORDER BY u.created_at DESC
	LIMIT $2 OFFSET $3;
	`
	rows, err := pool.Query(ctx, sqlQuery, search, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []UserRow{}
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.PlayerID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.Balance, &u.LavxBalance, &u.PrimeTier); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

type UserDetail struct {
	InternalID      int
	PlayerID        string
	Username        string
	IsAdmin         bool
	CreatedAt       time.Time
	Balance         float64
	LavxBalance     float64
	PrimeTier       string
	PrimeExpiresAt  *time.Time
	ReferralCode    string
	RefXp           int
	BattlepassTrack string
	BattlepassXp    int
	OpenPositions   int
	ClosedPositions int
	VoucherCount    int
	Statuses        []string
}

func GetUserDetail(ctx context.Context, pool *pgxpool.Pool, playerID string) (UserDetail, error) {
	var d UserDetail
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.player_id, u.username, u.is_admin, u.created_at, COALESCE(w.balance,0), COALESCE(w.lavx_balance,0)
		FROM users u LEFT JOIN wallets w ON w.user_id = u.id
		WHERE u.player_id = $1;
	`, playerID).Scan(&d.InternalID, &d.PlayerID, &d.Username, &d.IsAdmin, &d.CreatedAt, &d.Balance, &d.LavxBalance)
	if err != nil {
		return UserDetail{}, err
	}

	pool.QueryRow(ctx, `SELECT tier, expires_at FROM prime_subscriptions WHERE user_id = $1 AND tier IS NOT NULL AND expires_at > now();`, d.InternalID).Scan(&d.PrimeTier, &d.PrimeExpiresAt)

	err = pool.QueryRow(ctx, `SELECT referral_code, ref_xp FROM referrals WHERE user_id = $1;`, d.InternalID).Scan(&d.ReferralCode, &d.RefXp)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return UserDetail{}, err
	}

	var track *string
	pool.QueryRow(ctx, `SELECT track, xp FROM battlepass_progress WHERE user_id = $1;`, d.InternalID).Scan(&track, &d.BattlepassXp)
	if track != nil {
		d.BattlepassTrack = *track
	}

	pool.QueryRow(ctx, `SELECT COUNT(*) FROM positions WHERE user_id = $1 AND status = 'open';`, d.InternalID).Scan(&d.OpenPositions)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM positions WHERE user_id = $1 AND status = 'closed';`, d.InternalID).Scan(&d.ClosedPositions)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, d.InternalID).Scan(&d.VoucherCount)

	rows, err := pool.Query(ctx, `SELECT status FROM user_statuses WHERE user_id = $1;`, d.InternalID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s string
			rows.Scan(&s)
			d.Statuses = append(d.Statuses, s)
		}
	}

	return d, nil
}
