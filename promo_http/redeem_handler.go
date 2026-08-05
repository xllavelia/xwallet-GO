package promo_http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xwallet-server/auth_http"
	"xwallet-server/prime_sql"
	"xwallet-server/promo_sql"
	"xwallet-server/referral_sql"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type redeemRequest struct {
	Code string `json:"code"`
}

type redeemResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func RedeemHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		var req redeemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		code := strings.ToUpper(strings.TrimSpace(req.Code))
		if code == "" {
			http.Error(w, "enter a code", http.StatusBadRequest)
			return
		}

		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, authUser.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		if referral_sql.ReferralCodePattern.MatchString(code) {
			handleReferralRedeem(w, r, pool, code, userID)
			return
		}

		handlePromoRedeem(w, r, pool, code, userID)
	}
}

func handleReferralRedeem(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, code string, userID int) {
	referrerName, err := referral_sql.RedeemReferralCode(r.Context(), pool, code, userID)
	if err != nil {
		switch {
		case errors.Is(err, referral_sql.ErrReferralCodeNotFound):
			http.Error(w, "referral code not found", http.StatusNotFound)
		case errors.Is(err, referral_sql.ErrCannotReferSelf):
			http.Error(w, "you cannot use your own referral code", http.StatusBadRequest)
		case errors.Is(err, referral_sql.ErrAlreadyHasReferrer):
			http.Error(w, "you already have a referrer", http.StatusConflict)
		default:
			http.Error(w, "could not apply referral code", http.StatusInternalServerError)
		}
		return
	}

	writeSuccess(w, "referral", "Referral applied! @"+referrerName+" earned +25 XP")
}

func handlePromoRedeem(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, code string, userID int) {
	ctx := r.Context()

	maxSlots, err := prime_sql.GetMaxVoucherSlots(ctx, pool, userID)
	if err != nil {
		maxSlots = 5
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		http.Error(w, "could not start redemption", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	promo, err := promo_sql.LookupCodeForUpdate(ctx, tx, code)
	if err != nil {
		if errors.Is(err, promo_sql.ErrCodeNotFound) {
			http.Error(w, "code not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not look up code", http.StatusInternalServerError)
		return
	}

	if !promo.IsActive {
		http.Error(w, "this code is no longer active", http.StatusGone)
		return
	}
	if promo.ExpiresAt != nil && promo.ExpiresAt.Before(time.Now()) {
		http.Error(w, "this code has expired", http.StatusGone)
		return
	}
	if promo.MaxUses != nil {
		count, err := promo_sql.CountRedemptions(ctx, tx, promo.ID)
		if err != nil {
			http.Error(w, "could not verify code usage", http.StatusInternalServerError)
			return
		}
		if count >= *promo.MaxUses {
			http.Error(w, "this code has reached its usage limit", http.StatusGone)
			return
		}
	}

	isVoucherReward := promo.RewardType == "usdt_voucher" || promo.RewardType == "lavx_voucher" ||
		promo.RewardType == "ref_xp_voucher" || promo.RewardType == "fee_voucher"

	if isVoucherReward {
		slotCount, err := promo_sql.CountVoucherSlots(ctx, tx, userID)
		if err != nil {
			http.Error(w, "could not check voucher slots", http.StatusInternalServerError)
			return
		}
		if slotCount >= maxSlots {
			http.Error(w, fmt.Sprintf("your voucher slots are full (%d/%d) - clear one and try again", slotCount, maxSlots), http.StatusConflict)
			return
		}
	}

	recorded, err := promo_sql.TryRecordRedemption(ctx, tx, promo.ID, userID)
	if err != nil {
		http.Error(w, "could not record redemption", http.StatusInternalServerError)
		return
	}
	if !recorded {
		http.Error(w, "you've already redeemed this code", http.StatusConflict)
		return
	}

	if promo.RewardType == "usdt" {
		if _, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;`, promo.RewardValue, userID); err != nil {
			http.Error(w, "could not credit balance", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			http.Error(w, "could not finalize redemption", http.StatusInternalServerError)
			return
		}
		writeSuccess(w, "usdt", fmt.Sprintf("+$%.2f credited to your balance", promo.RewardValue))
		return
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "could not finalize redemption", http.StatusInternalServerError)
		return
	}

	var message string
	switch promo.RewardType {
	case "usdt_voucher":
		err = user_vouchers_sql.GrantCreditVoucher(ctx, pool, userID, "usdt_credit", promo.RewardValue, "promo", maxSlots)
		message = fmt.Sprintf("Voucher granted: $%.2f USDT credit - claim it in Vouchers", promo.RewardValue)
	case "lavx_voucher":
		err = user_vouchers_sql.GrantCreditVoucher(ctx, pool, userID, "lavx_credit", promo.RewardValue, "promo", maxSlots)
		message = fmt.Sprintf("Voucher granted: %.0f LAVX credit - claim it in Vouchers", promo.RewardValue)
	case "ref_xp_voucher":
		err = user_vouchers_sql.GrantCreditVoucher(ctx, pool, userID, "ref_xp_credit", promo.RewardValue, "promo", maxSlots)
		message = fmt.Sprintf("Voucher granted: +%.0f Referral XP - claim it in Vouchers", promo.RewardValue)
	case "fee_voucher":
		durationSeconds := 345600
		if promo.RewardDurationDays != nil {
			durationSeconds = *promo.RewardDurationDays * 86400
		}
		err = user_vouchers_sql.GrantFeeDiscountVoucher(ctx, pool, userID, promo.RewardValue, durationSeconds, "promo", maxSlots)
		message = fmt.Sprintf("Voucher granted: $%.2f fee-free trading - claim it in Vouchers", promo.RewardValue)
	default:
		http.Error(w, "this code has an unsupported reward type", http.StatusInternalServerError)
		return
	}

	if err != nil {
		if err == user_vouchers_sql.ErrSlotsFull {
			http.Error(w, "your voucher slots filled up during redemption - check Vouchers", http.StatusConflict)
			return
		}
		http.Error(w, "code redeemed, but the reward could not be granted - contact support", http.StatusInternalServerError)
		return
	}

	writeSuccess(w, promo.RewardType, message)
}

func writeSuccess(w http.ResponseWriter, rewardType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(redeemResponse{Type: rewardType, Message: message})
}
