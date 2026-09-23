package flip_engine

import (
	"context"
	cryptorand "crypto/rand"
	"log"
	"math/big"
	"sync"
	"time"

	"xwallet-server/flip_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var mu sync.RWMutex
var pool *pgxpool.Pool
var phase = "betting"
var nextRoundID int
var currentRoundID int
var phaseEndsAt time.Time
var currentOutcome string
var lastOutcome = flip_sql.ChoiceBlack

type PublicState struct {
	Phase            string
	RoundID          int
	SecondsRemaining float64
	Outcome          string
	LastOutcome      string
}

func Start(p *pgxpool.Pool) {
	pool = p

	outcome, err := flip_sql.GetLastOutcome(context.Background(), pool)
	if err != nil {
		log.Println("flip engine: could not restore last outcome:", err)
	} else {
		mu.Lock()
		lastOutcome = outcome
		mu.Unlock()
	}

	go runLoop()
	log.Println("flip engine started")
}

func runLoop() {
	ctx := context.Background()

	for {
		bettingEndsAt := time.Now().Add(flip_sql.BettingDuration)
		roundID, err := flip_sql.CreateRound(ctx, pool, bettingEndsAt)
		if err != nil {
			log.Println("flip engine: could not create round:", err)
			time.Sleep(2 * time.Second)
			continue
		}

		mu.Lock()
		phase = "betting"
		nextRoundID = roundID
		phaseEndsAt = bettingEndsAt
		currentOutcome = ""
		mu.Unlock()

		time.Sleep(time.Until(bettingEndsAt))

		outcome := generateOutcome()

		for {
			err = flip_sql.ResolveRound(ctx, pool, roundID, outcome)
			if err == nil || err == flip_sql.ErrRoundResolved {
				break
			}
			log.Println("flip engine: settlement failed:", err)
			time.Sleep(time.Second)
		}

		flippingEndsAt := time.Now().Add(flip_sql.FlipDuration)

		mu.Lock()
		phase = "flipping"
		currentRoundID = roundID
		currentOutcome = outcome
		phaseEndsAt = flippingEndsAt
		mu.Unlock()

		time.Sleep(time.Until(flippingEndsAt))

		for {
			err = flip_sql.FinishRound(ctx, pool, roundID)
			if err == nil {
				break
			}
			log.Println("flip engine: could not finish round:", err)
			time.Sleep(time.Second)
		}

		mu.Lock()
		lastOutcome = outcome
		mu.Unlock()
	}
}

func generateOutcome() string {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(2))
	if err != nil {
		log.Println("flip engine: crypto random failed, using secure fallback retry")
		for err != nil {
			value, err = cryptorand.Int(cryptorand.Reader, big.NewInt(2))
		}
	}

	if value.Int64() == 0 {
		return flip_sql.ChoiceRed
	}
	return flip_sql.ChoiceBlack
}

func GetPublicState() PublicState {
	mu.RLock()
	defer mu.RUnlock()

	remaining := time.Until(phaseEndsAt).Seconds()
	if remaining < 0 {
		remaining = 0
	}

	if phase == "flipping" {
		return PublicState{
			Phase:            phase,
			RoundID:          currentRoundID,
			SecondsRemaining: remaining,
			Outcome:          currentOutcome,
			LastOutcome:      lastOutcome,
		}
	}

	return PublicState{
		Phase:            "betting",
		RoundID:          nextRoundID,
		SecondsRemaining: remaining,
		LastOutcome:      lastOutcome,
	}
}

func PlaceBet(ctx context.Context, userID int, amount float64, choice string) (flip_sql.Bet, error) {
	mu.RLock()
	isBetting := phase == "betting"
	roundID := nextRoundID
	mu.RUnlock()

	if !isBetting {
		return flip_sql.Bet{}, flip_sql.ErrNotBettingPhase
	}

	return flip_sql.PlaceBet(ctx, pool, userID, roundID, amount, choice)
}
