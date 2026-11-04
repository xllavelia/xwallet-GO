package bankcards_http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type cardResponse struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	DesignID           int     `json:"designId"`
	CardNumber         string  `json:"cardNumber"`
	Balance            float64 `json:"balance"`
	IsActiveForTrading bool    `json:"isActiveForTrading"`
	OpenedAt           string  `json:"openedAt"`
}

type listResponse struct {
	Cards    []cardResponse `json:"cards"`
	MaxCards int            `json:"maxCards"`
}

func responseCard(c bankcards_sql.Card) cardResponse {
	return cardResponse{
		ID: c.ID, Name: c.Name, DesignID: c.DesignID, CardNumber: c.CardNumber,
		Balance: c.Balance, IsActiveForTrading: c.IsActiveForTrading,
		OpenedAt: c.OpenedAt.UTC().Format(time.RFC3339),
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func writeCardError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, bankcards_sql.ErrInvalidName), errors.Is(err, bankcards_sql.ErrInvalidDesign), errors.Is(err, bankcards_sql.ErrInvalidAmount):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, bankcards_sql.ErrMaxCardsReached), errors.Is(err, bankcards_sql.ErrCardFunded), errors.Is(err, bankcards_sql.ErrCardHasOpenPositions):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, bankcards_sql.ErrCardNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, bankcards_sql.ErrInsufficientFunds):
		writeError(w, http.StatusPaymentRequired, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}

func ListHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		cards, err := bankcards_sql.ListCards(r.Context(), pool, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load cards")
			return
		}
		items := make([]cardResponse, 0, len(cards))
		for _, c := range cards {
			items = append(items, responseCard(c))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listResponse{Cards: items, MaxCards: bankcards_sql.MaxCardsPerUser})
	}
}

type openRequest struct {
	Name     string `json:"name"`
	DesignID int    `json:"designId"`
}

func OpenHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req openRequest
		if err := decodeRequest(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name, err := bankcards_sql.ValidateOpenCard(req.Name, req.DesignID)
		if err != nil {
			writeCardError(w, err, "invalid account")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		card, err := bankcards_sql.OpenCard(r.Context(), pool, userID, name, req.DesignID)
		if err != nil {
			writeCardError(w, err, "could not open card")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseCard(card))
	}
}

type cardIDAmountRequest struct {
	CardID int     `json:"cardId"`
	Amount float64 `json:"amount"`
}

func TopUpHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req cardIDAmountRequest
		if err := decodeRequest(w, r, &req); err != nil || req.CardID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if err := bankcards_sql.ValidateCardAmount(req.Amount); err != nil {
			writeCardError(w, err, "invalid amount")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err := bankcards_sql.TopUpCard(r.Context(), pool, userID, req.CardID, req.Amount); err != nil {
			writeCardError(w, err, "could not top up card")
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
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req cardIDRequest
		if err := decodeRequest(w, r, &req); err != nil || req.CardID < 0 {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err := bankcards_sql.SelectActiveCard(r.Context(), pool, userID, req.CardID); err != nil {
			writeCardError(w, err, "could not select card")
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func CloseHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req cardIDRequest
		if err := decodeRequest(w, r, &req); err != nil || req.CardID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err := bankcards_sql.CloseCard(r.Context(), pool, userID, req.CardID); err != nil {
			writeCardError(w, err, "could not close card")
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func ResolveHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		number := r.URL.Query().Get("number")
		if len(number) != 16 {
			writeError(w, http.StatusBadRequest, "invalid card number")
			return
		}
		_, username, err := bankcards_sql.ResolveByCardNumber(r.Context(), pool, number)
		if err != nil {
			writeError(w, http.StatusNotFound, "card not found")
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
			writeError(w, http.StatusUnauthorized, "unauthorized")
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
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		results, err := bankcards_sql.SearchCardsByPrefix(r.Context(), pool, prefix, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search failed")
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

func WithdrawHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		authUser, ok := auth_http.UserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var req cardIDAmountRequest
		if err := decodeRequest(w, r, &req); err != nil || req.CardID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if err := bankcards_sql.ValidateCardAmount(req.Amount); err != nil {
			writeCardError(w, err, "invalid amount")
			return
		}
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err := bankcards_sql.WithdrawFromCard(r.Context(), pool, userID, req.CardID, req.Amount); err != nil {
			writeCardError(w, err, "could not withdraw from card")
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
