package p2p_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/p2p_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type merchantDTO struct {
	IsMerchant      bool    `json:"isMerchant"`
	DepositAmount   float64 `json:"depositAmount"`
	TotalDeals      int     `json:"totalDeals"`
	CompletedDeals  int     `json:"completedDeals"`
	SuccessRate     float64 `json:"successRate"`
	RequiredDeposit float64 `json:"requiredDeposit"`
}

func MerchantStatusHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserID(r, pool)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		m, err := p2p_sql.GetMerchantStatus(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load merchant status", http.StatusInternalServerError)
			return
		}
		resp := merchantDTO{RequiredDeposit: p2p_sql.DepositAmountUsd}
		if m != nil {
			resp.IsMerchant = true
			resp.DepositAmount = m.DepositAmount
			resp.TotalDeals = m.TotalDeals
			resp.CompletedDeals = m.CompletedDeals
			if m.TotalDeals > 0 {
				resp.SuccessRate = (float64(m.CompletedDeals) / float64(m.TotalDeals)) * 100
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func BecomeMerchantHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		err := p2p_sql.BecomeMerchant(r.Context(), pool, userID)
		if err != nil {
			switch err {
			case p2p_sql.ErrAlreadyMerchant:
				http.Error(w, "you're already a merchant", http.StatusConflict)
			case p2p_sql.ErrInsufficientDeposit:
				http.Error(w, "insufficient balance for the guarantee deposit", http.StatusPaymentRequired)
			default:
				http.Error(w, "could not become a merchant", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func WithdrawDepositHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		err := p2p_sql.WithdrawMerchantDeposit(r.Context(), pool, userID)
		if err != nil {
			if err == p2p_sql.ErrHasActiveListings {
				http.Error(w, "close all active listings first", http.StatusConflict)
				return
			}
			http.Error(w, "could not withdraw deposit", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
