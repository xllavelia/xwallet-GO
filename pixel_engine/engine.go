package pixel_engine

import (
	"context"
	"log"
	"sync"
	"time"

	"xwallet-server/pixel_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var mu sync.RWMutex
var pool *pgxpool.Pool
var phase = "betting"
var currentRoundID, nextRoundID int
var phaseEndsAt time.Time

func Start(p *pgxpool.Pool) {
	pool = p
	go runLoop()
	log.Println("pixel engine started")
}

func runLoop() {
	ctx := context.Background()
	for {
		mu.Lock()
		phase = "betting"
		roundID, err := pixel_sql.InsertRound(ctx, pool)
		if err != nil {
			log.Println("pixel engine: insert round failed:", err)
			mu.Unlock()
			time.Sleep(5 * time.Second)
			continue
		}
		nextRoundID = roundID
		phaseEndsAt = time.Now().Add(time.Duration(pixel_sql.BettingDuration) * time.Second)
		bettingEnd := phaseEndsAt
		mu.Unlock()

		time.Sleep(time.Until(bettingEnd))

		mu.Lock()
		phase = "running"
		currentRoundID = nextRoundID
		phaseEndsAt = time.Now().Add(time.Duration(pixel_sql.RoundDuration) * time.Second)
		roundID = currentRoundID
		runningEnd := phaseEndsAt
		mu.Unlock()

		pixel_sql.StartRound(ctx, pool, roundID)
		time.Sleep(time.Until(runningEnd))

		settleRound(pool, roundID)
		pixel_sql.EndRound(ctx, pool, roundID)
	}
}

func settleRound(pool *pgxpool.Pool, roundID int) {
	ctx := context.Background()
	boards, err := pixel_sql.GetActiveBoardsForRound(ctx, pool, roundID)
	if err != nil {
		return
	}
	for _, b := range boards {
		if err := pixel_sql.AutoCashOutBoard(ctx, pool, b); err != nil {
			log.Println("pixel engine: auto cashout failed for board", b.ID, err)
		}
	}
}

type PublicState struct {
	Phase            string
	RoundID          int
	SecondsRemaining float64
}

func GetPublicState() PublicState {
	mu.RLock()
	defer mu.RUnlock()
	if phase == "betting" {
		r := time.Until(phaseEndsAt).Seconds()
		if r < 0 {
			r = 0
		}
		return PublicState{Phase: "betting", RoundID: nextRoundID, SecondsRemaining: r}
	}
	r := time.Until(phaseEndsAt).Seconds()
	if r < 0 {
		r = 0
	}
	return PublicState{Phase: "running", RoundID: currentRoundID, SecondsRemaining: r}
}

func PlaceBet(ctx context.Context, userID int, amount float64) (int, error) {
	mu.RLock()
	isBetting := phase == "betting"
	roundID := nextRoundID
	mu.RUnlock()
	if !isBetting {
		return 0, pixel_sql.ErrNotBettingPhase
	}
	return pixel_sql.PlaceBet(ctx, pool, userID, roundID, amount)
}

func RevealCell(ctx context.Context, userID int, cellIndex int) (pixel_sql.RevealResult, error) {
	mu.RLock()
	isRunning := phase == "running"
	roundID := currentRoundID
	mu.RUnlock()
	if !isRunning {
		return pixel_sql.RevealResult{}, pixel_sql.ErrNotRunningPhase
	}
	return pixel_sql.RevealCell(ctx, pool, userID, roundID, cellIndex)
}

func CashOut(ctx context.Context, userID int) (float64, []int, error) {
	mu.RLock()
	isRunning := phase == "running"
	roundID := currentRoundID
	mu.RUnlock()
	if !isRunning {
		return 0, nil, pixel_sql.ErrNotRunningPhase
	}
	return pixel_sql.CashOut(ctx, pool, userID, roundID)
}

func GetMyBoard(ctx context.Context, userID int) (pixel_sql.BoardStateDTO, error) {
	mu.RLock()
	ph := phase
	var roundID int
	if ph == "betting" {
		roundID = nextRoundID
	} else {
		roundID = currentRoundID
	}
	mu.RUnlock()
	return pixel_sql.GetBoardState(ctx, pool, userID, roundID)
}
