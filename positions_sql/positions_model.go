package positions_sql

import "time"

type Position struct {
	ID                int
	TradeID           string
	UserID            int
	Coin              string
	Type              string
	EntryPrice        float64
	ClosePrice        *float64
	Leverage          int
	Amount            float64
	Margin            float64
	Fees              float64
	FeesPaidByVoucher bool
	LiqPrice          float64
	AutoClose         bool
	AutoCloseTarget   *float64
	Pnl               *float64
	PnlPercent        *float64
	Status            string
	Result            *string
	OpenedAt          time.Time
	ClosedAt          *time.Time
}
