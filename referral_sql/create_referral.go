package referral_sql

import (
	"context"
	"errors"
	"math/rand"

	"github.com/jackc/pgx/v5/pgxpool"
)

func generateReferralCode() string {
	letters := "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digits := "0123456789"
	b := make([]byte, 4)
	b[0] = letters[rand.Intn(len(letters))]
	b[1] = letters[rand.Intn(len(letters))]
	b[2] = digits[rand.Intn(len(digits))]
	b[3] = digits[rand.Intn(len(digits))]
	return "REF-" + string(b)
}

func codeExists(ctx context.Context, pool *pgxpool.Pool, code string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referral_code = $1);`, code).Scan(&exists)

	return exists, err
}

func CreateReferralForNewUser(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	for attempt := 0; attempt < 20; attempt++ {
		code := generateReferralCode()
		exists, err := codeExists(ctx, pool, code)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO referrals (user_id, referral_code)
			VALUES ($1, $2)
			ON CONFLICT (user_id) DO NOTHING;
		`, userID, code)
		return err
	}
	return errors.New("could not generate a unique referral code")
}
