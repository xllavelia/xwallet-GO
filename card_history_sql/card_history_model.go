package card_history_sql

import "time"

type CardHistoryEntry struct {
	ID            int
	UserID        int
	OperationType string
	FromAsset     string
	ToAsset       string
	FromAmount    float64
	ToAmount      float64
	Price         float64
	CreatedAt     time.Time
	XpAwarded     int
}
