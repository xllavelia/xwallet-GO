package bankcards_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type tierResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	OpenPriceUsd         float64 `json:"openPriceUsd"`
	CashbackPercent      float64 `json:"cashbackPercent"`
	CashbackPercentPrime float64 `json:"cashbackPercentPrime"`
	FeeReductionPoints   float64 `json:"feeReductionPoints"`
	FeeFullyWaived       bool    `json:"feeFullyWaived"`
	LavxPerMonth         float64 `json:"lavxPerMonth"`
	XpBonusPercent       float64 `json:"xpBonusPercent"`
	TransferFeePercent   float64 `json:"transferFeePercent"`
	PatternSeed          string  `json:"patternSeed"`
}

type cardResponse struct {
	ID                 int     `json:"id"`
	Tier               string  `json:"tier"`
	PatternSeed        string  `json:"patternSeed"`
	CardNumber         string  `json:"cardNumber"`
	Balance            float64 `json:"balance"`
	IsActiveForTrading bool    `json:"isActiveForTrading"`
	OpenedAt           string  `json:"openedAt"`
	CashbackThisMonth  float64 `json:"cashbackThisMonth"`
}

type listResponse struct {
	Cards                  []cardResponse `json:"cards"`
	Catalog                []tierResponse `json:"catalog"`
	MaxCards               int            `json:"maxCards"`
	TotalCashbackThisMonth float64        `json:"totalCashbackThisMonth"`
}

func ListHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		cards, err := bankcards_sql.ListCards(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load cards", http.StatusInternalServerError)
			return
		}

		cashbackByCard := map[int]float64{}
		var totalCashback float64
		rows, err := pool.Query(r.Context(), `
			SELECT funding_card_id, COALESCE(SUM(cashback_awarded),0)
			FROM positions
			WHERE user_id = $1 AND status = 'closed' AND funding_kind = 'card'
			  AND funding_card_id IS NOT NULL AND closed_at >= date_trunc('month', now())
			GROUP BY funding_card_id;
		`, userID)
		if err == nil {
			for rows.Next() {
				var cid int
				var amt float64
				rows.Scan(&cid, &amt)
				cashbackByCard[cid] = amt
				totalCashback += amt
			}
			rows.Close()
		}

		items := make([]cardResponse, 0, len(cards))
		for _, c := range cards {
			items = append(items, cardResponse{
				ID: c.ID, Tier: c.Tier, CardNumber: c.CardNumber, Balance: c.Balance,
				IsActiveForTrading: c.IsActiveForTrading, OpenedAt: c.OpenedAt.Format("2006-01-02T15:04:05Z"),
				CashbackThisMonth: cashbackByCard[c.ID],
			})
		}

		catalog := make([]tierResponse, 0, len(bankcards_sql.TierOrder))
		for _, id := range bankcards_sql.TierOrder {
			cfg := bankcards_sql.Tiers[id]
			catalog = append(catalog, tierResponse{
				ID: cfg.ID, Name: cfg.Name, OpenPriceUsd: cfg.OpenPriceUsd,
				CashbackPercent: cfg.CashbackPercent, CashbackPercentPrime: cfg.CashbackPercentPrime,
				FeeReductionPoints: cfg.FeeReductionPoints, FeeFullyWaived: cfg.FeeFullyWaived,
				LavxPerMonth: cfg.LavxPerMonth, XpBonusPercent: cfg.XpBonusPercent, PatternSeed: cfg.PatternSeed,
				TransferFeePercent: cfg.TransferFeePercent,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listResponse{
			Cards: items, Catalog: catalog, MaxCards: bankcards_sql.MaxCardsPerUser,
			TotalCashbackThisMonth: totalCashback,
		})
	}
}

type openRequest struct {
	Tier string `json:"tier"`
}

func OpenHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req openRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		card, err := bankcards_sql.OpenCard(r.Context(), pool, userID, req.Tier)
		if err != nil {
			switch err {
			case bankcards_sql.ErrTierNotFound:
				http.Error(w, "invalid tier", http.StatusBadRequest)
			case bankcards_sql.ErrTierAlreadyOwned:
				http.Error(w, "you already own this card tier", http.StatusConflict)
			case bankcards_sql.ErrInsufficientWalletBalance:
				http.Error(w, "insufficient wallet balance", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not open card", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cardResponse{
			ID: card.ID, Tier: card.Tier, CardNumber: card.CardNumber, Balance: card.Balance,
			IsActiveForTrading: card.IsActiveForTrading, OpenedAt: card.OpenedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}

type cardIDAmountRequest struct {
	CardID int     `json:"cardId"`
	Amount float64 `json:"amount"`
}

func TopUpHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req cardIDAmountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		err = bankcards_sql.TopUpCard(r.Context(), pool, userID, req.CardID, req.Amount)
		if err != nil {
			if err == bankcards_sql.ErrInsufficientFunds {
				http.Error(w, "insufficient wallet balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "could not top up card", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type cardIDRequest struct {
	CardID int `json:"cardId"`
}

func SelectActiveHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req cardIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := bankcards_sql.SelectActiveCard(r.Context(), pool, userID, req.CardID); err != nil {
			http.Error(w, "could not select card", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func CloseHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req cardIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := bankcards_sql.CloseCard(r.Context(), pool, userID, req.CardID); err != nil {
			http.Error(w, "could not close card", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func ResolveHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		number := r.URL.Query().Get("number")
		if len(number) != 16 {
			http.Error(w, "invalid card number", http.StatusBadRequest)
			return
		}
		_, username, err := bankcards_sql.ResolveByCardNumber(r.Context(), pool, number)
		if err != nil {
			http.Error(w, "card not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"username": username})
	}
}

type cardSearchItem struct {
	CardNumber string `json:"cardNumber"`
	Username   string `json:"username"`
	PlayerID   string `json:"playerId"`
	Tier       string `json:"tier"`
	IsOwnCard  bool   `json:"isOwnCard"`
}

func SearchHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		prefix := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		if len(prefix) == 0 {
			json.NewEncoder(w).Encode([]cardSearchItem{})
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		results, err := bankcards_sql.SearchCardsByPrefix(r.Context(), pool, prefix, userID)
		if err != nil {
			http.Error(w, "search failed", http.StatusInternalServerError)
			return
		}
		items := make([]cardSearchItem, 0, len(results))
		for _, res := range results {
			items = append(items, cardSearchItem{
				IsOwnCard: res.IsOwnCard, CardNumber: res.CardNumber, Username: res.Username, PlayerID: res.PlayerID, Tier: res.Tier,
			})
		}
		json.NewEncoder(w).Encode(items)
	}
}
