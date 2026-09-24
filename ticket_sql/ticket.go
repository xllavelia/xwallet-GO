package ticket_sql

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/bankcards_sql"
)

var (
	ErrUnknownRarity     = errors.New("unknown rarity")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrNoTickets         = errors.New("no tickets of this rarity left")
)

func randInt(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func rollTierValue(tiers []ValueTier) (float64, error) {
	total := 0
	for _, t := range tiers {
		total += t.Weight
	}
	pick, err := randInt(total)
	if err != nil {
		return 0, err
	}
	chosen := tiers[len(tiers)-1]
	for _, t := range tiers {
		if pick < t.Weight {
			chosen = t
			break
		}
		pick -= t.Weight
	}
	minCents := int(math.Round(chosen.Min * 100))
	maxCents := int(math.Round(chosen.Max * 100))
	if maxCents <= minCents {
		return float64(minCents) / 100, nil
	}
	cents, err := randInt(maxCents - minCents + 1)
	if err != nil {
		return 0, err
	}
	return float64(minCents+cents) / 100, nil
}

func BuyPack(ctx context.Context, pool *pgxpool.Pool, userID int, rarity string) error {
	pack, ok := PackByRarity(rarity)
	if !ok {
		return ErrUnknownRarity
	}
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, -pack.Price); err != nil {
		if errors.Is(err, bankcards_sql.ErrInsufficientFunds) {
			return ErrInsufficientFunds
		}
		return err
	}
	var packID int
	if err := tx.QueryRow(ctx, `
INSERT INTO ticket_packs (user_id, rarity, price) VALUES ($1, $2, $3) RETURNING id;
`, userID, pack.Rarity, pack.Price).Scan(&packID); err != nil {
		return err
	}
	for i := 0; i < pack.TicketCount; i++ {
		value, err := rollTierValue(pack.Tiers)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO tickets (pack_id, user_id, rarity, value, idx) VALUES ($1, $2, $3, $4, $5);
`, packID, userID, pack.Rarity, value, i); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO ticket_user_stats (user_id, packs_opened, total_spent) VALUES ($1, 1, $2)
ON CONFLICT (user_id) DO UPDATE SET packs_opened = ticket_user_stats.packs_opened + 1,
total_spent = ticket_user_stats.total_spent + $2, updated_at = now();
`, userID, pack.Price); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type OpenResult struct {
	TicketID  int     `json:"ticketId"`
	PackID    int     `json:"packId"`
	Rarity    string  `json:"rarity"`
	Value     float64 `json:"value"`
	Remaining int     `json:"remaining"`
}

func OpenTicket(ctx context.Context, pool *pgxpool.Pool, userID int, rarity string) (OpenResult, error) {
	if _, ok := PackByRarity(rarity); !ok {
		return OpenResult{}, ErrUnknownRarity
	}
	fundingSource, err := bankcards_sql.ResolveFundingSource(ctx, pool, userID)
	if err != nil {
		return OpenResult{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return OpenResult{}, err
	}
	defer tx.Rollback(ctx)
	var ticketID, packID int
	var value float64
	err = tx.QueryRow(ctx, `
SELECT id, pack_id, value FROM tickets
WHERE user_id = $1 AND rarity = $2 AND revealed = FALSE
ORDER BY pack_id, idx LIMIT 1 FOR UPDATE;
`, userID, rarity).Scan(&ticketID, &packID, &value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OpenResult{}, ErrNoTickets
		}
		return OpenResult{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE tickets SET revealed = TRUE, revealed_at = now() WHERE id = $1;
`, ticketID); err != nil {
		return OpenResult{}, err
	}
	if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, value); err != nil {
		return OpenResult{}, err
	}
	var remaining int
	if err := tx.QueryRow(ctx, `
SELECT COUNT(*) FROM tickets WHERE user_id = $1 AND rarity = $2 AND revealed = FALSE;
`, userID, rarity).Scan(&remaining); err != nil {
		return OpenResult{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO ticket_user_stats (user_id, tickets_opened, total_won, best_ticket)
VALUES ($1, 1, $2, $2)
ON CONFLICT (user_id) DO UPDATE SET tickets_opened = ticket_user_stats.tickets_opened + 1,
total_won = ticket_user_stats.total_won + $2,
best_ticket = GREATEST(ticket_user_stats.best_ticket, $2), updated_at = now();
`, userID, value); err != nil {
		return OpenResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OpenResult{}, err
	}
	return OpenResult{TicketID: ticketID, PackID: packID, Rarity: rarity, Value: value, Remaining: remaining}, nil
}

type PackInfo struct {
	Rarity      string  `json:"rarity"`
	DisplayName string  `json:"displayName"`
	Price       float64 `json:"price"`
	TicketCount int     `json:"ticketCount"`
	Unopened    int     `json:"unopened"`
	NextTicket  int     `json:"nextTicket"`
	NextPack    int     `json:"nextPack"`
	NextIndex   int     `json:"nextIndex"`
}

type UserStatsDTO struct {
	PacksOpened   int     `json:"packsOpened"`
	TicketsOpened int     `json:"ticketsOpened"`
	TotalSpent    float64 `json:"totalSpent"`
	TotalWon      float64 `json:"totalWon"`
	BestTicket    float64 `json:"bestTicket"`
	NetProfit     float64 `json:"netProfit"`
}

type GlobalStatsDTO struct {
	TotalPacks    int     `json:"totalPacks"`
	TotalTickets  int     `json:"totalTickets"`
	BiggestTicket float64 `json:"biggestTicket"`
}

type TicketState struct {
	Packs       []PackInfo     `json:"packs"`
	MyStats     UserStatsDTO   `json:"myStats"`
	GlobalStats GlobalStatsDTO `json:"globalStats"`
}

func GetState(ctx context.Context, pool *pgxpool.Pool, userID int) (TicketState, error) {
	state := TicketState{Packs: []PackInfo{}}
	for _, p := range Packs {
		state.Packs = append(state.Packs, PackInfo{
			Rarity:      p.Rarity,
			DisplayName: p.DisplayName,
			Price:       p.Price,
			TicketCount: p.TicketCount,
		})
	}
	rows, err := pool.Query(ctx, `
SELECT rarity, COUNT(*) FROM tickets WHERE user_id = $1 AND revealed = FALSE GROUP BY rarity;
`, userID)
	if err != nil {
		return state, err
	}
	counts := map[string]int{}
	for rows.Next() {
		var rarity string
		var c int
		if err := rows.Scan(&rarity, &c); err != nil {
			rows.Close()
			return state, err
		}
		counts[rarity] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return state, err
	}
	for i := range state.Packs {
		p := &state.Packs[i]
		p.Unopened = counts[p.Rarity]
		if p.Unopened == 0 {
			continue
		}
		var id, packID, idx int
		if err := pool.QueryRow(ctx, `
SELECT id, pack_id, idx FROM tickets
WHERE user_id = $1 AND rarity = $2 AND revealed = FALSE
ORDER BY pack_id, idx LIMIT 1;
`, userID, p.Rarity).Scan(&id, &packID, &idx); err != nil {
			return state, err
		}
		p.NextTicket = id
		p.NextPack = packID
		p.NextIndex = idx + 1
	}
	us := UserStatsDTO{}
	err = pool.QueryRow(ctx, `
SELECT packs_opened, tickets_opened, total_spent, total_won, best_ticket
FROM ticket_user_stats WHERE user_id = $1;
`, userID).Scan(&us.PacksOpened, &us.TicketsOpened, &us.TotalSpent, &us.TotalWon, &us.BestTicket)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return state, err
	}
	us.NetProfit = us.TotalWon - us.TotalSpent
	state.MyStats = us
	gs := GlobalStatsDTO{}
	if err := pool.QueryRow(ctx, `
SELECT
(SELECT COUNT(*) FROM ticket_packs),
(SELECT COUNT(*) FROM tickets WHERE revealed = TRUE),
(SELECT COALESCE(MAX(value), 0) FROM tickets WHERE revealed = TRUE);
`).Scan(&gs.TotalPacks, &gs.TotalTickets, &gs.BiggestTicket); err != nil {
		return state, err
	}
	state.GlobalStats = gs
	return state, nil
}
