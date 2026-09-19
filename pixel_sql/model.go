package pixel_sql

import (
	"errors"
	"time"
)

var ErrAlreadyBet = errors.New("you already placed a bet for the next round")
var ErrNotBettingPhase = errors.New("betting is closed for this round")
var ErrInsufficientFunds = errors.New("insufficient balance")
var ErrNoActiveBoard = errors.New("no active board")
var ErrNotRunningPhase = errors.New("game is not currently running")
var ErrCellAlreadyRevealed = errors.New("this cell is already revealed")
var ErrInvalidCell = errors.New("invalid cell index")

type Round struct {
	ID        int
	StartedAt *time.Time
	EndedAt   *time.Time
	CreatedAt time.Time
}

type Board struct {
	ID             int
	RoundID        int
	UserID         int
	Amount         float64
	GridCols       int
	GridRows       int
	MinePositions  []int
	RevealedCells  []int
	LivesRemaining int
	Status         string
	Payout         *float64
	CreatedAt      time.Time
	EndedAt        *time.Time
}
