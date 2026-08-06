package battlepass_sql

import "time"

type Progress struct {
	UserID           int
	Track            *string
	Xp               int
	ClaimedLevels    []int
	EpicCases        int
	MythicCases      int
	LegendaryCases   int
	LastTransferXpAt *time.Time
	LastCardBuyXpAt  *time.Time
	LastCardSellXpAt *time.Time
}
