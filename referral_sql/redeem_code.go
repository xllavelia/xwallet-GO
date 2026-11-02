package referral_sql

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrReferralCodeNotFound = errors.New("referral code not found")
var ErrCannotReferSelf = errors.New("cannot use your own referral code")
var ErrAlreadyHasReferrer = errors.New("user already has a referrer")

func RedeemReferralCode(ctx context.Context, pool *pgxpool.Pool, code string, referredUserID int) (int, string, error) {
	var referrerUserID int
	var referrerUsername string
	err := pool.QueryRow(ctx, `
SELECT r.user_id, u.username
FROM referrals r
JOIN users u ON u.id = r.user_id
WHERE r.referral_code = $1;
`, code).Scan(&referrerUserID, &referrerUsername)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", ErrReferralCodeNotFound
		}
		return 0, "", err
	}
	if referrerUserID == referredUserID {
		return 0, "", ErrCannotReferSelf
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
INSERT INTO referral_links (referrer_user_id, referred_user_id)
VALUES ($1, $2)
ON CONFLICT (referred_user_id) DO NOTHING;
`, referrerUserID, referredUserID)
	if err != nil {
		return 0, "", err
	}
	if tag.RowsAffected() == 0 {
		return 0, "", ErrAlreadyHasReferrer
	}
	if _, err := tx.Exec(ctx, `UPDATE referrals SET ref_xp = ref_xp + 25 WHERE user_id = $1;`, referrerUserID); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return referrerUserID, referrerUsername, nil
}
