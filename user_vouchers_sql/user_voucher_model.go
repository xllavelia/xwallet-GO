package user_vouchers_sql

import "time"

type UserVoucher struct {
	ID              int
	UserID          int
	VoucherType     string
	Status          string
	LimitAmount     *float64
	UsedAmount      float64
	DurationSeconds *int
	ActivatedAt     *time.Time
	CreditAmount    *float64
	Source          string
	CreatedAt       time.Time
}
