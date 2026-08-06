package positions_http

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/positions_sql"
	"xwallet-server/prime_sql"
	"xwallet-server/referral_sql"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"
	"xwallet-server/wallet_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type openPositionRequest struct {
	Coin            string   `json:"coin"`
	Type            string   `json:"type"`
	EntryPrice      float64  `json:"entryPrice"`
	Leverage        int      `json:"leverage"`
	Amount          float64  `json:"amount"`
	AutoClose       bool     `json:"autoClose"`
	AutoCloseTarget *float64 `json:"autoCloseTarget"`
}

type positionResponse struct {
	ID                int      `json:"id"`
	TradeID           string   `json:"tradeId"`
	Coin              string   `json:"coin"`
	Type              string   `json:"type"`
	EntryPrice        float64  `json:"entryPrice"`
	Leverage          int      `json:"leverage"`
	Amount            float64  `json:"amount"`
	Margin            float64  `json:"margin"`
	Fees              float64  `json:"fees"`
	FeesFromVoucher   float64  `json:"feesFromVoucher"`
	FeesFromBalance   float64  `json:"feesFromBalance"`
	FeesPaidByVoucher bool     `json:"feesPaidByVoucher"`
	LiqPrice          float64  `json:"liqPrice"`
	AutoClose         bool     `json:"autoClose"`
	AutoCloseTarget   *float64 `json:"autoCloseTarget"`
	OpenedAt          string   `json:"openedAt"`
}

func generateTradeID() string {
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, 6)
	for i := range out {
		out[i] = chars[rand.Intn(len(chars))]
	}
	return "TRD-" + string(out)
}

func OpenPositionHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req openPositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Type != "long" && req.Type != "short" {
			http.Error(w, "type must be 'long' or 'short'", http.StatusBadRequest)
			return
		}
		if req.Amount <= 0 {
			http.Error(w, "amount must be positive", http.StatusBadRequest)
			return
		}
		if req.Leverage <= 0 || req.Leverage > 200 {
			http.Error(w, "leverage must be between 1 and 200", http.StatusBadRequest)
			return
		}
		if req.EntryPrice <= 0 {
			http.Error(w, "invalid entry price", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		_, feeRate, err := prime_sql.GetEffectiveFeeRate(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not determine fee rate", http.StatusInternalServerError)
			return
		}

		boostPoints, _ := user_vouchers_sql.GetActiveFeeBoostPoints(r.Context(), pool, userID)
		feeRate -= boostPoints / 10
		if feeRate < 0 {
			feeRate = 0
		}

		margin := req.Amount / float64(req.Leverage)
		fees := CalcFeesOnMargin(margin, feeRate)
		liqPrice := CalcLiquidationPrice(req.EntryPrice, req.Leverage, req.Type)

		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "could not start transaction", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context())

		feesFromVoucher := 0.0
		voucherID := 0
		voucher, voucherErr := user_vouchers_sql.FindActiveFeeVoucherForUpdate(r.Context(), tx, userID)
		if voucherErr == nil {
			remaining := voucher.LimitAmount - voucher.UsedAmount
			if remaining > 0 {
				if fees <= remaining {
					feesFromVoucher = fees
				} else {
					feesFromVoucher = remaining
				}
				voucherID = voucher.ID
			}
		}
		feesFromBalance := fees - feesFromVoucher
		feesPaidByVoucher := feesFromVoucher > 0 && feesFromBalance <= 0

		totalRequired := margin + feesFromBalance

		pos := positions_sql.Position{
			TradeID: generateTradeID(), UserID: userID, Coin: req.Coin, Type: req.Type,
			EntryPrice: req.EntryPrice, Leverage: req.Leverage, Amount: req.Amount, Margin: margin,
			Fees: fees, FeesPaidByVoucher: feesPaidByVoucher, LiqPrice: liqPrice,
			AutoClose: req.AutoClose, AutoCloseTarget: req.AutoCloseTarget,
		}

		created, err := positions_sql.InsertPositionTx(r.Context(), tx, pos)
		if err != nil {
			http.Error(w, "could not open position", http.StatusInternalServerError)
			return
		}

		if err := wallet_sql.AdjustBalanceTx(r.Context(), tx, userID, -totalRequired); err != nil {
			if errors.Is(err, wallet_sql.ErrInsufficientBalanceTx) {
				http.Error(w, "insufficient balance", http.StatusPaymentRequired)
				return
			}
			http.Error(w, "could not update balance", http.StatusInternalServerError)
			return
		}

		if voucherID != 0 && feesFromVoucher > 0 {
			if err := user_vouchers_sql.ConsumeVoucherAmount(r.Context(), tx, voucherID, feesFromVoucher); err != nil {
				http.Error(w, "could not apply voucher", http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "could not finalize trade", http.StatusInternalServerError)
			return
		}

		referral_sql.CreditReferrerIfAny(r.Context(), pool, userID, feesFromBalance, req.Amount)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(positionResponse{
			ID: created.ID, TradeID: created.TradeID, Coin: created.Coin, Type: created.Type,
			EntryPrice: created.EntryPrice, Leverage: created.Leverage, Amount: created.Amount,
			Margin: created.Margin, Fees: fees, FeesFromVoucher: feesFromVoucher, FeesFromBalance: feesFromBalance,
			FeesPaidByVoucher: feesPaidByVoucher, LiqPrice: created.LiqPrice,
			AutoClose: created.AutoClose, AutoCloseTarget: created.AutoCloseTarget,
			OpenedAt: created.OpenedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
}
