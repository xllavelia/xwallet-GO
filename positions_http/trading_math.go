package positions_http

var TRADE_FEE_RATE = 0.01 // 1% от margin

func CalcLiquidationPrice(entryPrice float64, leverage int, positionType string) float64 {
	if positionType == "short" {
		return entryPrice * (1 + 1/float64(leverage))
	}
	return entryPrice * (1 - 1/float64(leverage))
}

func CalcFeesOnMargin(margin float64) float64 {
	return margin * TRADE_FEE_RATE
}

func CalcPnl(margin float64, leverage int, entryPrice float64, currentPrice float64, positionType string) float64 {
	if entryPrice <= 0 {
		return 0
	}
	direction := 1.0
	if positionType == "short" {
		direction = -1.0
	}
	priceMove := currentPrice - entryPrice
	pnl := margin * float64(leverage) * (priceMove / entryPrice) * direction

	maxLoss := -margin
	if pnl < maxLoss {
		return maxLoss
	}
	return pnl
}

func CalcPnlPercent(pnl float64, margin float64) float64 {
	if margin <= 0 {
		return 0
	}
	return (pnl / margin) * 100
}
