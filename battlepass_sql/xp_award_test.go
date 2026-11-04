package battlepass_sql

import "testing"

func TestTradeBaseXPUnchanged(t *testing.T) {
	for _, tc := range []struct {
		pnl float64
		xp  int
	}{
		{0, 0}, {9.99, 0}, {10, 100}, {99.99, 100}, {100, 300}, {1000, 300},
		{-0.99, 0}, {-1, 100}, {-9.99, 100}, {-10, 300}, {-1000, 300},
	} {
		if got := tradeBaseXP(tc.pnl); got != tc.xp {
			t.Errorf("pnl %v: XP %d; want %d", tc.pnl, got, tc.xp)
		}
	}
}
