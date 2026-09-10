package p2p_http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"xwallet-server/p2p_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type dealDTO struct {
	ID               int     `json:"id"`
	ListingID        int     `json:"listingId"`
	Role             string  `json:"role"`
	Side             string  `json:"side"`
	BaseAsset        string  `json:"baseAsset"`
	AssetClass       string  `json:"assetClass"`
	BaseAmount       float64 `json:"baseAmount"`
	QuoteAmountUsd   float64 `json:"quoteAmountUsd"`
	RateUsd          float64 `json:"rateUsd"`
	CommissionUsd    float64 `json:"commissionUsd"`
	Status           string  `json:"status"`
	ExpiresAt        string  `json:"expiresAt"`
	CompletedAt      string  `json:"completedAt"`
	CreatedAt        string  `json:"createdAt"`
	CounterpartyName string  `json:"counterpartyName"`
}

func toDealDTO(d p2p_sql.DealWithParty, viewerUserID int) dealDTO {
	role := "taker"
	counterparty := d.PosterUsername
	if d.PosterUserID == viewerUserID {
		role = "poster"
		counterparty = d.TakerUsername
	}
	completedAt := ""
	if d.CompletedAt != nil {
		completedAt = d.CompletedAt.Format("2006-01-02T15:04:05Z")
	}
	return dealDTO{
		ID: d.ID, ListingID: d.ListingID, Role: role, Side: d.Side, BaseAsset: d.BaseAsset, AssetClass: d.AssetClass,
		BaseAmount: d.BaseAmount, QuoteAmountUsd: d.QuoteAmountUsd, RateUsd: d.RateUsd, CommissionUsd: d.CommissionUsd,
		Status: d.Status, ExpiresAt: d.ExpiresAt.Format("2006-01-02T15:04:05Z"), CompletedAt: completedAt,
		CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z"), CounterpartyName: counterparty,
	}
}

type createDealRequest struct {
	ListingID      int     `json:"listingId"`
	QuoteAmountUsd float64 `json:"quoteAmountUsd"`
}

func CreateDealHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req createDealRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		dealID, err := p2p_sql.CreateDeal(r.Context(), pool, userID, req.ListingID, req.QuoteAmountUsd)
		if err != nil {
			switch err {
			case p2p_sql.ErrListingNotFound:
				http.Error(w, "listing not found or no longer active", http.StatusNotFound)
			case p2p_sql.ErrCannotDealOwnListing:
				http.Error(w, "cannot start a deal on your own listing", http.StatusBadRequest)
			case p2p_sql.ErrAmountOutOfRange:
				http.Error(w, "amount is outside the listing's limits", http.StatusBadRequest)
			case p2p_sql.ErrListingInsufficientVolume:
				http.Error(w, "not enough volume left in this listing", http.StatusConflict)
			default:
				http.Error(w, "could not create deal", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"dealId": dealID})
	}
}

type dealIDRequest struct {
	DealID int `json:"dealId"`
}

func ConfirmDealHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req dealIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		err := p2p_sql.ConfirmDeal(r.Context(), pool, userID, req.DealID)
		if err != nil {
			switch err {
			case p2p_sql.ErrDealNotFound:
				http.Error(w, "deal not found", http.StatusNotFound)
			case p2p_sql.ErrDealNotPending:
				http.Error(w, "this deal is no longer pending", http.StatusConflict)
			case p2p_sql.ErrDealExpired:
				http.Error(w, "this deal has expired", http.StatusConflict)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func CancelDealHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		var req dealIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := p2p_sql.CancelDeal(r.Context(), pool, userID, req.DealID); err != nil {
			http.Error(w, "could not cancel deal", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func MyDealsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		activeOnly := r.URL.Query().Get("status") == "active"
		deals, err := p2p_sql.GetMyDeals(r.Context(), pool, userID, activeOnly)
		if err != nil {
			http.Error(w, "could not load deals", http.StatusInternalServerError)
			return
		}
		items := make([]dealDTO, 0, len(deals))
		for _, d := range deals {
			items = append(items, toDealDTO(d, userID))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

func DealDetailHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "invalid deal id", http.StatusBadRequest)
			return
		}
		d, err := p2p_sql.GetDealDetail(r.Context(), pool, userID, id)
		if err != nil {
			http.Error(w, "deal not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toDealDTO(d, userID))
	}
}
