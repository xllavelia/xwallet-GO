package positions_engine

import (
	"context"
	"log"
	"time"

	"xwallet-server/bankcards_sql"
	"xwallet-server/battlepass_sql"
	"xwallet-server/positions_sql"
	"xwallet-server/priceoracle"
	"xwallet-server/prime_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartTimeTradeSettler(pool *pgxpool.Pool) {
	go func() {
		for {
			settleExpiredTimeTrades(pool)
			time.Sleep(10 * time.Second)
		}
	}()
	log.Println("time trade settler started (every 10s)")
}

func settleExpiredTimeTrades(pool *pgxpool.Pool) {
	ctx := context.Background()
	ids, err := positions_sql.GetExpiredOpenTimeTradeIDs(ctx, pool)
	if err != nil || len(ids) == 0 {
		return
	}
	for _, id := range ids {
		settleOne(pool, id)
	}
}

func settleOne(pool *pgxpool.Pool, id int) {
	ctx := context.Background()

	pos, err := positions_sql.GetPositionByID(ctx, pool, id)
	if err != nil || pos.Status != "open" || pos.TradeMode != "time" {
		return
	}

	currentPrice, has := priceoracle.Get(pos.Coin)
	if !has || currentPrice <= 0 {
		return // будет подхвачено на следующем тике
	}

	isWin := false
	if pos.Type == "long" {
		isWin = currentPrice > pos.EntryPrice
	} else {
		isWin = currentPrice < pos.EntryPrice
	}

	multiplier := 1.0
	if pos.PayoutMultiplier != nil {
		multiplier = *pos.PayoutMultiplier
	}

	var pnl, payoutTotal float64
	var result string
	if isWin {
		pnl = pos.Margin * (multiplier - 1)
		result = "win"
		payoutTotal = pos.Margin + pnl
	} else {
		pnl = -pos.Margin
		result = "loss"
		payoutTotal = 0
	}
	pnlPercent := 0.0
	if pos.Margin > 0 {
		pnlPercent = (pnl / pos.Margin) * 100
	}

	fundingSource := bankcards_sql.FundingSource{Kind: "wallet", UserID: pos.UserID}
	if pos.FundingKind == "card" && pos.FundingCardID != nil {
		fundingSource = bankcards_sql.FundingSource{Kind: "card", CardID: *pos.FundingCardID}
	}

	cashback := 0.0
	if isWin && fundingSource.Kind == "card" {
		if cardTier, tierErr := bankcards_sql.GetCardTier(ctx, pool, fundingSource.CardID); tierErr == nil {
			if cfg, ok := bankcards_sql.Tiers[cardTier]; ok {
				rate := cfg.CashbackPercent
				if cardTier == "saint" {
					if primeSub, _ := prime_sql.GetActiveSubscription(ctx, pool, pos.UserID); primeSub != nil {
						rate = cfg.CashbackPercentPrime
					}
				}
				cashback = pnl * (rate / 100)
			}
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	if err := positions_sql.ClosePositionTx(ctx, tx, id, currentPrice, pnl, pnlPercent, result, cashback); err != nil {
		log.Println("time trade settle: close failed for", id, err)
		return
	}
	if payoutTotal+cashback > 0 {
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, payoutTotal+cashback); err != nil {
			log.Println("time trade settle: payout failed for", id, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Println("time trade settle: commit failed for", id, err)
		return
	}

	xp := battlepass_sql.AwardTradeXP(ctx, pool, pos.UserID, pnl, fundingSource)
	if xp > 0 {
		positions_sql.SetXpAwarded(ctx, pool, id, xp)
	}
}
