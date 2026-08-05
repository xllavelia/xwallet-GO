package wallet_sql

type Wallet struct {
	UserID            int
	Balance           float64
	LavxBalance       float64
	Profit24h         float64
	Profit7d          float64
	ActiveTradesCount int
	WinRate           float64
}
