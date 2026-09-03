package card_http

import "xwallet-server/priceoracle"

func fetchLivePrices(coins []string) (map[string]float64, error) {
	return priceoracle.GetAll(coins), nil
}
