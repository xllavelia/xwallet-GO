package stocks_sql

import "errors"

var ErrInvalidSymbol = errors.New("invalid symbol")
var ErrInsufficientFunds = errors.New("insufficient funds")
var ErrNoHolding = errors.New("no holding for this symbol")

type Holding struct {
	Symbol   string
	Quantity float64
	AvgCost  float64
}
