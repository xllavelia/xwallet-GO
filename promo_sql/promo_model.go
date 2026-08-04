package promo_sql

import "time"

type PromoCode struct {
	ID                 int
	Code               string
	RewardType         string
	RewardValue        float64
	RewardDurationDays *int
	MaxUses            *int
	IsActive           bool
	ExpiresAt          *time.Time
	CreatedAt          time.Time
}
