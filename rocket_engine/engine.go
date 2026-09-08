package rocket_engine

import (
	"context"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"xwallet-server/rocket_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

const BettingDuration = 20 * time.Second
const GrowthRate = 0.09
const StartMultiplier = 0.5
const MaxMultiplier = 5.0

var mu sync.RWMutex
var pool *pgxpool.Pool

var phase = "betting"
var currentRoundID int
var nextRoundID int
var crashPoint float64
var phaseEndsAt time.Time
var roundStartedAt time.Time

func multiplierAt(elapsed time.Duration) float64 {
	t := elapsed.Seconds()
	if t < 0 {
		t = 0
	}
	m := StartMultiplier * math.Exp(GrowthRate*t)
	if m > MaxMultiplier {
		m = MaxMultiplier
	}
	return m
}

func timeToReach(target float64) time.Duration {
	if target <= StartMultiplier {
		return 0
	}
	t := math.Log(target/StartMultiplier) / GrowthRate
	return time.Duration(t * float64(time.Second))
}

func generateCrashPoint() float64 {
	u := rand.Float64()
	if u < 0.03 {
		return StartMultiplier
	}
	houseEdge := 0.05
	raw := (1 - houseEdge) / (1 - u)
	if raw < StartMultiplier {
		raw = StartMultiplier
	}
	if raw > MaxMultiplier {
		raw = MaxMultiplier
	}
	return math.Round(raw*100) / 100
}

func Start(p *pgxpool.Pool) {
	pool = p
	go runLoop()
	log.Println("rocket engine started")
}

func runLoop() {
	ctx := context.Background()

	for {
		mu.Lock()
		phase = "betting"
		cp := generateCrashPoint()
		roundID, err := rocket_sql.InsertRound(ctx, pool, cp)
		if err != nil {
			log.Println("rocket engine: could not insert round:", err)
			mu.Unlock()
			time.Sleep(5 * time.Second)
			continue
		}
		nextRoundID = roundID
		crashPoint = cp
		phaseEndsAt = time.Now().Add(BettingDuration)
		bettingEnd := phaseEndsAt
		mu.Unlock()

		time.Sleep(time.Until(bettingEnd))

		mu.Lock()
		phase = "running"
		currentRoundID = nextRoundID
		roundStartedAt = time.Now()
		crashDuration := timeToReach(crashPoint)
		phaseEndsAt = roundStartedAt.Add(crashDuration)
		roundID = currentRoundID
		crashAt := phaseEndsAt
		mu.Unlock()

		rocket_sql.StartRound(ctx, pool, roundID)
		time.Sleep(time.Until(crashAt))

		mu.Lock()
		rocket_sql.SettleLostBets(ctx, pool, roundID)
		rocket_sql.EndRound(ctx, pool, roundID)
		mu.Unlock()
	}
}

type PublicState struct {
	Phase             string
	RoundID           int
	SecondsRemaining  float64
	CurrentMultiplier float64
}

func GetPublicState() PublicState {
	mu.RLock()
	defer mu.RUnlock()

	if phase == "betting" {
		remaining := time.Until(phaseEndsAt).Seconds()
		if remaining < 0 {
			remaining = 0
		}
		return PublicState{Phase: "betting", RoundID: nextRoundID, SecondsRemaining: remaining, CurrentMultiplier: StartMultiplier}
	}

	elapsed := time.Since(roundStartedAt)
	m := multiplierAt(elapsed)
	remaining := time.Until(phaseEndsAt).Seconds()
	if remaining < 0 {
		remaining = 0
	}
	return PublicState{Phase: "running", RoundID: currentRoundID, SecondsRemaining: remaining, CurrentMultiplier: m}
}

func PlaceBet(ctx context.Context, userID int, amount float64) (int, error) {
	mu.RLock()
	isBetting := phase == "betting"
	roundID := nextRoundID
	mu.RUnlock()

	if !isBetting {
		return 0, rocket_sql.ErrNotBettingPhase
	}
	return rocket_sql.PlaceBet(ctx, pool, userID, roundID, amount)
}

func CashOut(ctx context.Context, userID int) (float64, float64, error) {
	mu.RLock()
	isRunning := phase == "running"
	roundID := currentRoundID
	startedAt := roundStartedAt
	crashAt := phaseEndsAt
	mu.RUnlock()

	if !isRunning {
		return 0, 0, rocket_sql.ErrRoundNotRunning
	}
	now := time.Now()
	if !now.Before(crashAt) {
		return 0, 0, rocket_sql.ErrTooLate
	}
	m := multiplierAt(now.Sub(startedAt))
	payout, err := rocket_sql.CashOutBet(ctx, pool, userID, roundID, m)
	if err != nil {
		return 0, 0, err
	}
	return m, payout, nil
}
