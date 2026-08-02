package card_sql

import "time"

type CryptoCard struct {
	ID         int
	CardNumber string
	UserID     int
	BtcAmount  float64
	EthAmount  float64
	SolAmount  float64
	TonAmount  float64
	ValidThru  time.Time
	CreatedAt  time.Time
}
