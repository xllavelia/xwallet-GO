package bankcards_sql

import "time"

type Card struct {
	ID                 int
	UserID             int
	Tier               string
	CardNumber         string
	Balance            float64
	IsActiveForTrading bool
	LastLavxGrantAt    *time.Time
	OpenedAt           time.Time
}

type FundingSource struct {
	Kind   string // "card" | "wallet"
	CardID int
	UserID int
}
