package positions_sql

var MinTimeTradeSeconds = 5 * 60
var MaxTimeTradeSeconds = 72 * 60 * 60

func PayoutMultiplierForDuration(seconds int) float64 {
	minutes := seconds / 60
	switch {
	case minutes < 30:
		return 2.80
	case minutes < 120:
		return 2.85
	case minutes < 720:
		return 2.90
	default:
		return 2.95
	}
}
