package wallet_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/prime_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type walletResponse struct {
	Balance           float64 `json:"balance"`
	LavxBalance       float64 `json:"lavxBalance"`
	Profit24h         float64 `json:"profit24h"`
	Profit7d          float64 `json:"profit7d"`
	ActiveTradesCount int     `json:"activeTradesCount"`
	WinRate           float64 `json:"winRate"`
	PrimeTier         string  `json:"primeTier"`
	FeeRatePercent    float64 `json:"feeRatePercent"`
	MaxVoucherSlots   int     `json:"maxVoucherSlots"`
}

func GetWalletHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		wallet, err := wallet_sql.GetWalletByPlayerID(r.Context(), pool, user.PlayerID)
		if err != nil {
			internalID, idErr := users_sql.GetInternalIDByPlayerID(r.Context(), pool, user.PlayerID)
			if idErr != nil {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			if err := wallet_sql.InsertWallet(r.Context(), pool, internalID); err != nil {
				http.Error(w, "could not create wallet", http.StatusInternalServerError)
				return
			}
			wallet, err = wallet_sql.GetWalletByPlayerID(r.Context(), pool, user.PlayerID)
			if err != nil {
				http.Error(w, "could not load wallet", http.StatusInternalServerError)
				return
			}
		}

		tier, feeRate, err := prime_sql.GetEffectiveFeeRate(r.Context(), pool, wallet.UserID)
		if err != nil {
			feeRate = prime_sql.BaseFeeRatePercent
		}

		maxSlots, err := prime_sql.GetMaxVoucherSlots(r.Context(), pool, wallet.UserID)
		if err != nil {
			maxSlots = prime_sql.DefaultVoucherSlots
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(walletResponse{
			Balance: wallet.Balance, LavxBalance: wallet.LavxBalance,
			Profit24h: wallet.Profit24h, Profit7d: wallet.Profit7d,
			ActiveTradesCount: wallet.ActiveTradesCount, WinRate: wallet.WinRate,
			PrimeTier: tier, FeeRatePercent: feeRate, MaxVoucherSlots: maxSlots,
		})
	}
}
