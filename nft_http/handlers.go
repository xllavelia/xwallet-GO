package nft_http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/auth_http"
	"xwallet-server/nft_sql"
	"xwallet-server/users_sql"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func userID(r *http.Request, pool *pgxpool.Pool) (int, bool) {
	u, ok := auth_http.UserFromContext(r)
	if !ok {
		return 0, false
	}
	id, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, u.PlayerID)
	return id, err == nil
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, nft_sql.ErrNotFound), errors.Is(err, nft_sql.ErrListingGone):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, nft_sql.ErrNotOwner), errors.Is(err, nft_sql.ErrOwnListing):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, nft_sql.ErrListed), errors.Is(err, nft_sql.ErrNotListed),
		errors.Is(err, nft_sql.ErrSoldOut), errors.Is(err, nft_sql.ErrDropClosed),
		errors.Is(err, nft_sql.ErrPriceChanged):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, nft_sql.ErrInsufficientFunds), errors.Is(err, nft_sql.ErrInvalidPrice):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("nft: %v", err)
		writeError(w, http.StatusInternalServerError, "nft service error")
	}
}

type body struct {
	ItemID     int64   `json:"itemId"`
	ListingID  int64   `json:"listingId"`
	TemplateID string  `json:"templateId"`
	PackID     string  `json:"packId"`
	Price      float64 `json:"price"`
	MaxPrice   float64 `json:"maxPrice"`
}

// handle объединяет общую обвязку: метод, пользователь, тело и ленивое создание дропов.
func handle(pool *pgxpool.Pool, method string, fn func(r *http.Request, uid int, b body) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		uid, ok := userID(r, pool)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var b body
		if method == http.MethodPost {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
				writeError(w, http.StatusBadRequest, "invalid body")
				return
			}
		}
		if err := nft_sql.Ensure(r.Context(), pool); err != nil {
			fail(w, err)
			return
		}
		out, err := fn(r, uid, b)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func StateHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodGet, func(r *http.Request, uid int, _ body) (any, error) {
		return nft_sql.GetState(r.Context(), pool, uid)
	})
}

func MarketHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodGet, func(r *http.Request, uid int, _ body) (any, error) {
		return nft_sql.GetMarket(r.Context(), pool, uid)
	})
}

func CollectHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, _ body) (any, error) {
		return nft_sql.Collect(r.Context(), pool, uid)
	})
}

func SellHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		return nft_sql.InstantSell(r.Context(), pool, uid, b.ItemID)
	})
}

func ListHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		return nft_sql.List(r.Context(), pool, uid, b.ItemID, b.Price)
	})
}

func UnlistHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		if err := nft_sql.Unlist(r.Context(), pool, uid, b.ItemID); err != nil {
			return nil, err
		}
		return nft_sql.ItemByID(r.Context(), pool, b.ItemID)
	})
}

func BuyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		return nft_sql.BuyListing(r.Context(), pool, uid, b.ListingID)
	})
}

func GameBuyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		return nft_sql.BuyGame(r.Context(), pool, uid, b.TemplateID, b.MaxPrice)
	})
}

func PackBuyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return handle(pool, http.MethodPost, func(r *http.Request, uid int, b body) (any, error) {
		return nft_sql.BuyPack(r.Context(), pool, uid, b.PackID)
	})
}
