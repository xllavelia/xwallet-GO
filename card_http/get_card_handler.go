package card_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/card_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type assetResponse struct {
	ID         string  `json:"id"`
	Amount     float64 `json:"amount"`
	ValueUsd   float64 `json:"valueUsd"`
	Allocation float64 `json:"allocation"`
}

type cardResponse struct {
	CardNumber  string          `json:"cardNumber"`
	Holder      string          `json:"holder"`
	ValidThru   string          `json:"validThru"`
	BalanceUsd  float64         `json:"balanceUsd"`
	Assets      []assetResponse `json:"assets"`
	UsdtBalance float64         `json:"usdtBalance"`
}

var coinOrder = []string{"BTC", "ETH", "SOL", "TON"}

func GetCardHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		card, err := card_sql.GetCardByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			if err := card_sql.InsertCard(r.Context(), pool, userID); err != nil {
				http.Error(w, "could not create card", http.StatusInternalServerError)
				return
			}
			card, err = card_sql.GetCardByPlayerID(r.Context(), pool, authUser.PlayerID)
			if err != nil {
				http.Error(w, "could not load card", http.StatusInternalServerError)
				return
			}
		}

		prices, err := fetchLivePrices(coinOrder)
		if err != nil {
			http.Error(w, "could not fetch live prices", http.StatusInternalServerError)
			return
		}

		amounts := map[string]float64{
			"BTC": card.BtcAmount, "ETH": card.EthAmount, "SOL": card.SolAmount, "TON": card.TonAmount,
		}

		fundingSource, fsErr := bankcards_sql.ResolveFundingSource(r.Context(), pool, userID)
		usdtBalance := 0.0
		if fsErr == nil {
			usdtBalance, _ = bankcards_sql.GetFundingBalance(r.Context(), pool, fundingSource)
		}

		cryptoUsd := 0.0
		values := map[string]float64{}
		for _, coin := range coinOrder {
			v := amounts[coin] * prices[coin]
			values[coin] = v
			cryptoUsd += v
		}
		totalUsd := cryptoUsd + usdtBalance

		assets := make([]assetResponse, 0, 4)
		for _, coin := range coinOrder {
			allocation := 0.0
			if totalUsd > 0 {
				allocation = (values[coin] / totalUsd) * 100
			}
			assets = append(assets, assetResponse{
				ID: coin, Amount: amounts[coin], ValueUsd: values[coin], Allocation: allocation,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cardResponse{
			CardNumber:  card.CardNumber,
			Holder:      authUser.Username,
			ValidThru:   card.ValidThru.Format("01/06"),
			BalanceUsd:  totalUsd,
			Assets:      assets,
			UsdtBalance: usdtBalance,
		})
	}
}
