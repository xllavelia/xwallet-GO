package battlepass_sql

import (
	"context"
	"errors"
	"math/rand"

	"xwallet-server/user_vouchers_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoCasesAvailable = errors.New("no cases of this rarity available")

type RolledVoucher struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value"`
	Days  int     `json:"days"`
}

type CaseResult struct {
	Rarity        string          `json:"rarity"`
	UsdtAwarded   float64         `json:"usdtAwarded"`
	LavxAwarded   float64         `json:"lavxAwarded"`
	Vouchers      []RolledVoucher `json:"vouchers"`
	StatusGranted string          `json:"statusGranted"`
}

type caseConfig struct {
	usdtMin, usdtMax int
	lavxMin, lavxMax int
	voucherCount     int
	refXpMax         float64
	feeMax           float64
	bothPerVoucher   bool
	statusChance     float64
	statusName       string
	column           string
}

var caseConfigs = map[string]caseConfig{
	"epic":      {10, 30, 1, 5, 2, 25, 50, false, 0.03, "lucky", "epic_cases"},
	"mythic":    {20, 100, 5, 10, 3, 35, 100, false, 0.03, "young", "mythic_cases"},
	"legendary": {50, 200, 10, 20, 5, 25, 50, true, 0.03, "saint", "legendary_cases"},
}

func randInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func OpenCase(ctx context.Context, pool *pgxpool.Pool, userID int, rarity string, maxSlots int) (CaseResult, error) {
	cfg, ok := caseConfigs[rarity]
	if !ok {
		return CaseResult{}, errors.New("invalid rarity")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return CaseResult{}, err
	}
	defer tx.Rollback(ctx)

	var remaining int
	sqlQuery := `
		UPDATE battlepass_progress SET ` + cfg.column + ` = ` + cfg.column + ` - 1
		WHERE user_id = $1 AND ` + cfg.column + ` > 0
		RETURNING ` + cfg.column + `;
	`
	if err := tx.QueryRow(ctx, sqlQuery, userID).Scan(&remaining); err != nil {
		return CaseResult{}, ErrNoCasesAvailable
	}

	result := CaseResult{Rarity: rarity}
	result.UsdtAwarded = float64(randInt(cfg.usdtMin, cfg.usdtMax))
	result.LavxAwarded = float64(randInt(cfg.lavxMin, cfg.lavxMax))

	if _, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, lavx_balance = lavx_balance + $2, updated_at = now() WHERE user_id = $3;`, result.UsdtAwarded, result.LavxAwarded, userID); err != nil {
		return CaseResult{}, err
	}

	for i := 0; i < cfg.voucherCount; i++ {
		grantRefXp := rand.Intn(2) == 0
		if cfg.bothPerVoucher {
			refAmt := float64(randInt(1, int(cfg.refXpMax)))
			feeAmt := float64(randInt(5, int(cfg.feeMax)))
			if err := user_vouchers_sql.GrantCreditVoucherTx(ctx, tx, userID, "ref_xp_credit", refAmt, "xdrop_"+rarity, maxSlots); err == nil {
				result.Vouchers = append(result.Vouchers, RolledVoucher{Kind: "ref_xp_credit", Value: refAmt})
			}
			if err := user_vouchers_sql.GrantFeeDiscountVoucherTx(ctx, tx, userID, feeAmt, 4*86400, "xdrop_"+rarity, maxSlots); err == nil {
				result.Vouchers = append(result.Vouchers, RolledVoucher{Kind: "fee_discount", Value: feeAmt, Days: 4})
			}
		} else if grantRefXp {
			refAmt := float64(randInt(1, int(cfg.refXpMax)))
			if err := user_vouchers_sql.GrantCreditVoucherTx(ctx, tx, userID, "ref_xp_credit", refAmt, "xdrop_"+rarity, maxSlots); err == nil {
				result.Vouchers = append(result.Vouchers, RolledVoucher{Kind: "ref_xp_credit", Value: refAmt})
			}
		} else {
			feeAmt := float64(randInt(5, int(cfg.feeMax)))
			if err := user_vouchers_sql.GrantFeeDiscountVoucherTx(ctx, tx, userID, feeAmt, 4*86400, "xdrop_"+rarity, maxSlots); err == nil {
				result.Vouchers = append(result.Vouchers, RolledVoucher{Kind: "fee_discount", Value: feeAmt, Days: 4})
			}
		}
	}

	if rand.Float64() < cfg.statusChance {
		if _, err := tx.Exec(ctx, `INSERT INTO user_statuses (user_id, status) VALUES ($1, $2) ON CONFLICT (user_id, status) DO NOTHING;`, userID, cfg.statusName); err == nil {
			result.StatusGranted = cfg.statusName
		}
	}

	return result, tx.Commit(ctx)
}
