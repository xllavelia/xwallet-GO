package user_vouchers_sql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetByUserID(ctx context.Context, pool *pgxpool.Pool, userID int) ([]UserVoucher, error) {
	sqlQuery := `
	SELECT id, user_id, voucher_type, status, limit_amount, used_amount, duration_seconds, activated_at, credit_amount, source, created_at
	FROM user_vouchers
	WHERE user_id = $1
	ORDER BY created_at ASC
	LIMIT 5;
	`
	rows, err := pool.Query(ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []UserVoucher{}
	for rows.Next() {
		var v UserVoucher
		err := rows.Scan(&v.ID, &v.UserID, &v.VoucherType, &v.Status, &v.LimitAmount, &v.UsedAmount, &v.DurationSeconds, &v.ActivatedAt, &v.CreditAmount, &v.Source, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}

	return result, rows.Err()
}
