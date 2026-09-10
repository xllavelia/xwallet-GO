package p2p_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/p2p_sql"
	"xwallet-server/priceoracle"
	"xwallet-server/stockoracle"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type listingDTO struct {
	ID                  int     `json:"id"`
	Side                string  `json:"side"`
	BaseAsset           string  `json:"baseAsset"`
	AssetClass          string  `json:"assetClass"`
	RateUsd             float64 `json:"rateUsd"`
	MinAmountUsd        float64 `json:"minAmountUsd"`
	MaxAmountUsd        float64 `json:"maxAmountUsd"`
	RemainingBaseAmount float64 `json:"remainingBaseAmount"`
	PaymentNote         string  `json:"paymentNote"`
	Status              string  `json:"status"`
	CreatedAt           string  `json:"createdAt"`
	Username            string  `json:"username"`
	PlayerID            string  `json:"playerId"`
	IsOfficial          bool    `json:"isOfficial"`
	TotalDeals          int     `json:"totalDeals"`
	CompletedDeals      int     `json:"completedDeals"`
	SuccessRate         float64 `json:"successRate"`
}

func toListingDTO(l p2p_sql.ListingWithPoster) listingDTO {
	successRate := 0.0
	if l.TotalDeals > 0 {
		successRate = (float64(l.CompletedDeals) / float64(l.TotalDeals)) * 100
	}
	return listingDTO{
		ID: l.ID, Side: l.Side, BaseAsset: l.BaseAsset, AssetClass: l.AssetClass,
		RateUsd: l.RateUsd, MinAmountUsd: l.MinAmountUsd, MaxAmountUsd: l.MaxAmountUsd,
		RemainingBaseAmount: l.RemainingBaseAmount, PaymentNote: l.PaymentNote, Status: l.Status,
		CreatedAt: l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Username:  l.Username, PlayerID: l.PlayerID, IsOfficial: l.IsOfficial,
		TotalDeals: l.TotalDeals, CompletedDeals: l.CompletedDeals, SuccessRate: successRate,
	}
}

func getUserID(r *http.Request, pool *pgxpool.Pool) (int, bool) {
	authUser, ok := auth_http.UserFromContext(r)
	if !ok {
		return 0, false
	}
	userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func ListListingsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		side := r.URL.Query().Get("side")
		if side != "buy" && side != "sell" {
			http.Error(w, "side must be 'buy' or 'sell'", http.StatusBadRequest)
			return
		}
		assetClass := r.URL.Query().Get("assetClass")
		asset := r.URL.Query().Get("asset")
		amountUsd := 0.0
		json.Unmarshal([]byte(r.URL.Query().Get("amountUsd")), &amountUsd)
		officialOnly := r.URL.Query().Get("official") == "true"

		listings, err := p2p_sql.GetActiveListings(r.Context(), pool, side, assetClass, asset, amountUsd, officialOnly, userID)
		if err != nil {
			http.Error(w, "could not load listings", http.StatusInternalServerError)
			return
		}
		items := make([]listingDTO, 0, len(listings))
		for _, l := range listings {
			items = append(items, toListingDTO(l))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

func MyListingsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		listings, err := p2p_sql.GetMyListings(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load listings", http.StatusInternalServerError)
			return
		}
		items := make([]listingDTO, 0, len(listings))
		for _, l := range listings {
			items = append(items, toListingDTO(p2p_sql.ListingWithPoster{Listing: l}))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

type createListingRequest struct {
	Side         string  `json:"side"`
	AssetClass   string  `json:"assetClass"`
	Asset        string  `json:"asset"`
	RateUsd      float64 `json:"rateUsd"`
	MinAmountUsd float64 `json:"minAmountUsd"`
	MaxAmountUsd float64 `json:"maxAmountUsd"`
	BaseAmount   float64 `json:"baseAmount"`
	PaymentNote  string  `json:"paymentNote"`
}

func CreateListingHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req createListingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		id, err := p2p_sql.CreateListing(r.Context(), pool, userID, req.Side, req.AssetClass, req.Asset, req.RateUsd, req.MinAmountUsd, req.MaxAmountUsd, req.BaseAmount, req.PaymentNote)
		if err != nil {
			switch err {
			case p2p_sql.ErrNotMerchant:
				http.Error(w, "become a merchant first", http.StatusForbidden)
			case p2p_sql.ErrInvalidListing:
				http.Error(w, "invalid listing parameters", http.StatusBadRequest)
			case p2p_sql.ErrInsufficientAsset:
				http.Error(w, "insufficient balance to back this listing", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not create listing", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"id": id})
	}
}

type listingIDRequest struct {
	ListingID int `json:"listingId"`
}

func CloseListingHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req listingIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := p2p_sql.CloseListing(r.Context(), pool, userID, req.ListingID); err != nil {
			http.Error(w, "could not close listing", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func MarketPriceHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		assetClass := r.URL.Query().Get("assetClass")
		asset := r.URL.Query().Get("asset")
		price := 0.0
		if assetClass == "crypto" {
			if p, has := priceoracle.Get(asset); has {
				price = p
			}
		} else if assetClass == "stock" {
			quotes := stockoracle.GetAll()
			if q, has := quotes[asset]; has {
				price = q.Price
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]float64{"price": price})
	}
}
