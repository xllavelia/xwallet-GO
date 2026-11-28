package referral_sql

import (
	"context"
	"errors"
	"math/rand"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

type referralDB interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func codeExists(ctx context.Context, db referralDB, code string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referral_code = $1);`, code).Scan(&exists)
	return exists, err
}

func CreateReferralForNewUser(ctx context.Context, pool *pgxpool.Pool, userID int) error {
	return createReferralForNewUser(ctx, pool, userID)
}

func CreateReferralForNewUserTx(ctx context.Context, tx pgx.Tx, userID int) error {
	return createReferralForNewUser(ctx, tx, userID)
}

func createReferralForNewUser(ctx context.Context, db referralDB, userID int) error {
	for attempt := 0; attempt < 20; attempt++ {
		code := generateReferralCode()
		exists, err := codeExists(ctx, db, code)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		_, err = db.Exec(ctx, `
			INSERT INTO referrals (user_id, referral_code)
			VALUES ($1, $2)
			ON CONFLICT (user_id) DO NOTHING;
		`, userID, code)
		return err
	}
	return errors.New("could not generate a unique referral code")
}
