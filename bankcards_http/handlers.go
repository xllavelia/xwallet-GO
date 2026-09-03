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
}

type cardResponse struct {
	ID                 int     `json:"id"`
	Tier               string  `json:"tier"`
	CardNumber         string  `json:"cardNumber"`
	Balance            float64 `json:"balance"`
	IsActiveForTrading bool    `json:"isActiveForTrading"`
	OpenedAt           string  `json:"openedAt"`
}

type listResponse struct {
	Cards    []cardResponse `json:"cards"`
	Catalog  []tierResponse `json:"catalog"`
	MaxCards int            `json:"maxCards"`
}

func toCardResponse(c bankcards_sql.Card) cardResponse {
	return cardResponse{
		ID: c.ID, Tier: c.Tier, CardNumber: c.CardNumber, Balance: c.Balance,
		IsActiveForTrading: c.IsActiveForTrading, OpenedAt: c.OpenedAt.Format("2006-01-02T15:04:05Z"),
	}
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

		items := make([]cardResponse, 0, len(cards))
		for _, c := range cards {
			items = append(items, toCardResponse(c))
		}

		catalog := make([]tierResponse, 0, len(bankcards_sql.TierOrder))
		for _, id := range bankcards_sql.TierOrder {
			cfg := bankcards_sql.Tiers[id]
			catalog = append(catalog, tierResponse{
				ID: cfg.ID, Name: cfg.Name, OpenPriceUsd: cfg.OpenPriceUsd,
				CashbackPercent: cfg.CashbackPercent, CashbackPercentPrime: cfg.CashbackPercentPrime,
				FeeReductionPoints: cfg.FeeReductionPoints, FeeFullyWaived: cfg.FeeFullyWaived,
				LavxPerMonth: cfg.LavxPerMonth, XpBonusPercent: cfg.XpBonusPercent,
				TransferFeePercent: cfg.TransferFeePercent,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listResponse{Cards: items, Catalog: catalog, MaxCards: bankcards_sql.MaxCardsPerUser})
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
			case bankcards_sql.ErrMaxCardsReached:
				http.Error(w, "maximum of 3 cards reached", http.StatusConflict)
			case bankcards_sql.ErrInsufficientWalletBalance:
				http.Error(w, "insufficient wallet balance", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not open card", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toCardResponse(card))
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
