package positions_http

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

func StartLiquidationWorker(pool *pgxpool.Pool) {
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		for range ticker.C {
			runLiquidationCheck(pool)
		}
	}()
	log.Println("liquidation worker started (every 15s, card-aware)")
}

func runLiquidationCheck(pool *pgxpool.Pool) {
	ctx := context.Background()

	positions, err := positions_sql.GetAllOpenPositions(ctx, pool)
	if err != nil || len(positions) == 0 {
		return
	}

	for _, p := range positions {
		if p.TradeMode != "standard" {
			continue // time trades settle через отдельный воркер
		}

		currentPrice, hasPrice := priceoracle.Get(p.Coin)
		if !hasPrice || currentPrice <= 0 {
			continue
		}

		shouldLiquidate := false
		if p.Type == "long" && currentPrice <= p.LiqPrice {
			shouldLiquidate = true
		}
		if p.Type == "short" && currentPrice >= p.LiqPrice {
			shouldLiquidate = true
		}

		pnl := CalcPnl(p.Margin, p.Leverage, p.EntryPrice, currentPrice, p.Type)
		pnlPercent := CalcPnlPercent(pnl, p.Margin)
		shouldAutoClose := p.AutoClose && p.AutoCloseTarget != nil && pnlPercent >= *p.AutoCloseTarget

		if !shouldLiquidate && !shouldAutoClose {
			continue
		}

		closePrice := currentPrice
		if shouldLiquidate {
			closePrice = p.LiqPrice
		}
		result := "win"
		if pnl < 0 {
			result = "loss"
		}

		fundingSource := bankcards_sql.FundingSource{Kind: "wallet", UserID: p.UserID}
		if p.FundingKind == "card" && p.FundingCardID != nil {
			fundingSource = bankcards_sql.FundingSource{Kind: "card", CardID: *p.FundingCardID}
		}

		cashback := 0.0
		if pnl > 0 && fundingSource.Kind == "card" {
			if cardTier, tierErr := bankcards_sql.GetCardTier(ctx, pool, fundingSource.CardID); tierErr == nil {
				if cfg, ok := bankcards_sql.Tiers[cardTier]; ok {
					rate := cfg.CashbackPercent
					if cardTier == "saint" {
						if primeSub, _ := prime_sql.GetActiveSubscription(ctx, pool, p.UserID); primeSub != nil {
							rate = cfg.CashbackPercentPrime
						}
					}
					cashback = pnl * (rate / 100)
				}
			}
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			continue
		}

		if err := positions_sql.ClosePositionTx(ctx, tx, p.ID, closePrice, pnl, pnlPercent, result, cashback); err != nil {
			tx.Rollback(ctx)
			log.Println("liquidation worker: close failed for", p.ID, err)
			continue
		}
		if err := bankcards_sql.AdjustFundingBalance(ctx, tx, fundingSource, p.Margin+pnl+cashback); err != nil {
			tx.Rollback(ctx)
			log.Println("liquidation worker: payout failed for", p.ID, err)
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			log.Println("liquidation worker: commit failed for", p.ID, err)
			continue
		}

		xp := battlepass_sql.AwardTradeXP(ctx, pool, p.UserID, pnl, fundingSource)
		if xp > 0 {
			positions_sql.SetXpAwarded(ctx, pool, p.ID, xp)
		}

		if shouldLiquidate {
			log.Println("liquidated position", p.TradeID)
		} else {
			log.Println("auto-closed position", p.TradeID, "at target profit")
		}
	}
}
