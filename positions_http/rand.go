package positions_http

import "math/rand"

func randIndex(max int) int {
	return rand.Intn(max)
}
