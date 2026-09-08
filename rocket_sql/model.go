package rocket_sql

import (
	"errors"
	"time"
)

var ErrAlreadyBet = errors.New("you already placed a bet for the next round")
var ErrNotBettingPhase = errors.New("betting is closed for this round")
var ErrInsufficientFunds = errors.New("insufficient balance")
var ErrNoActiveBet = errors.New("no active bet to cash out")
var ErrTooLate = errors.New("round already ended")
var ErrRoundNotRunning = errors.New("no round is currently running")

type Round struct {
	ID         int
	CrashPoint float64
	StartedAt  *time.Time
	EndedAt    *time.Time
	CreatedAt  time.Time
}

type Bet struct {
	ID                int
	RoundID           int
	UserID            int
	Amount            float64
	Status            string
	CashoutMultiplier *float64
	Payout            *float64
	CreatedAt         time.Time
}
