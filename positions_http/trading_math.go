package positions_http

func CalcLiquidationPrice(entryPrice float64, leverage int, positionType string) float64 {
	if positionType == "short" {
		return entryPrice * (1 + 1/float64(leverage))
	}
	return entryPrice * (1 - 1/float64(leverage))
}

func CalcFees(amount float64) float64 {
	return amount * 0.005
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
