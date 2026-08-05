package prime_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tierResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	MonthlyPriceLavx float64 `json:"monthlyPriceLavx"`
	AnnualPriceLavx  float64 `json:"annualPriceLavx"`
	FeeRatePercent   float64 `json:"feeRatePercent"`
	FeeFree          bool    `json:"feeFree"`
	MaxVoucherSlots  int     `json:"maxVoucherSlots"`
	UsdVoucherAmount float64 `json:"usdVoucherAmount"`
	FeeVoucherLimit  float64 `json:"feeVoucherLimit"`
	FeeVoucherDays   int     `json:"feeVoucherDays"`
	RefXpVoucher     float64 `json:"refXpVoucher"`
}

type primeStatusResponse struct {
	LavxBalance   float64        `json:"lavxBalance"`
	ActiveTier    string         `json:"activeTier"`
	ActiveBilling string         `json:"activeBilling"`
	ExpiresAt     string         `json:"expiresAt"`
	BaseFeeRate   float64        `json:"baseFeeRate"`
	Tiers         []tierResponse `json:"tiers"`
}

func GetPrimeStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		var lavxBalance float64
		if err := pool.QueryRow(r.Context(), `SELECT lavx_balance FROM wallets WHERE user_id = $1;`, userID).Scan(&lavxBalance); err != nil {
			http.Error(w, "could not load wallet", http.StatusInternalServerError)
			return
		}

		sub, err := prime_sql.GetActiveSubscription(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load subscription", http.StatusInternalServerError)
			return
		}

		resp := primeStatusResponse{
			LavxBalance: lavxBalance,
			BaseFeeRate: prime_sql.BaseFeeRatePercent,
			Tiers:       []tierResponse{},
		}
		if sub != nil {
			resp.ActiveTier = sub.Tier
			resp.ActiveBilling = sub.Billing
			resp.ExpiresAt = sub.ExpiresAt.Format("2006-01-02T15:04:05Z")
		}

		for _, id := range prime_sql.TierOrder() {
			cfg := prime_sql.Tiers[id]
			resp.Tiers = append(resp.Tiers, tierResponse{
				ID: cfg.ID, Name: cfg.Name,
				MonthlyPriceLavx: cfg.MonthlyPriceLavx, AnnualPriceLavx: cfg.AnnualPriceLavx,
				FeeRatePercent: cfg.FeeRatePercent, FeeFree: cfg.FeeFree,
				MaxVoucherSlots: cfg.MaxVoucherSlots, UsdVoucherAmount: cfg.UsdVoucherAmount,
				FeeVoucherLimit: cfg.FeeVoucherLimit, FeeVoucherDays: cfg.FeeVoucherDays,
				RefXpVoucher: cfg.RefXpVoucher,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
