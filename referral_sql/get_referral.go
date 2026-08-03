package referral_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (Referral, error) {
	var r Referral
	err := pool.QueryRow(ctx, `
		SELECT user_id, referral_code, ref_xp, total_referred_volume, total_earned
		FROM referrals WHERE user_id = $1;
	`, userID).Scan(&r.UserID, &r.ReferralCode, &r.RefXp, &r.TotalReferredVolume, &r.TotalEarned)

	return r, err
}

func GetOrCreateByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) (Referral, error) {
	r, err := getByUserID(ctx, pool, userID)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Referral{}, err
	}
	if err := CreateReferralForNewUser(ctx, pool, userID); err != nil {
		return Referral{}, err
	}
	return getByUserID(ctx, pool, userID)
}
