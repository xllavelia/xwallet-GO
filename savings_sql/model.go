package savings_sql

import "time"

var AnnualInterestRatePercent = 12.0

type Account struct {
	UserID        int
	Balance       float64
	LastAccruedAt time.Time
}

type HistoryEntry struct {
	ID        int
	UserID    int
	EntryType string
	Amount    float64
	CreatedAt time.Time
}
