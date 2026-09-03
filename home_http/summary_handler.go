package home_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type category struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

type summaryResponse struct {
	Categories               []category `json:"categories"`
	TotalIncome              float64    `json:"totalIncome"`
	TotalExpense             float64    `json:"totalExpense"`
	SavingsInterestThisMonth float64    `json:"savingsInterestThisMonth"`
}

func GetSummaryHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		ctx := r.Context()

		var tradesVol, tradesIncome, tradesExpense float64
		pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(ABS(pnl)),0), COALESCE(SUM(GREATEST(pnl,0)),0), COALESCE(SUM(GREATEST(-pnl,0)),0)
			FROM positions WHERE user_id=$1 AND status='closed' AND closed_at >= date_trunc('month', now());
		`, userID).Scan(&tradesVol, &tradesIncome, &tradesExpense)

		var transfersVol, transfersIncome, transfersExpense float64
		pool.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(amount + CASE WHEN sender_id=$1 THEN fee_amount ELSE 0 END),0),
				COALESCE(SUM(CASE WHEN recipient_id=$1 THEN amount ELSE 0 END),0),
				COALESCE(SUM(CASE WHEN sender_id=$1 THEN amount + fee_amount ELSE 0 END),0)
			FROM transfers WHERE (sender_id=$1 OR recipient_id=$1) AND created_at >= date_trunc('month', now());
		`, userID).Scan(&transfersVol, &transfersIncome, &transfersExpense)

		var cardVol, cardIncome, cardExpense float64
		pool.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(CASE WHEN operation_type='buy' THEN from_amount WHEN operation_type='sell' THEN to_amount ELSE 0 END),0),
				COALESCE(SUM(CASE WHEN operation_type='sell' THEN to_amount ELSE 0 END),0),
				COALESCE(SUM(CASE WHEN operation_type='buy' THEN from_amount ELSE 0 END),0)
			FROM card_history WHERE user_id=$1 AND created_at >= date_trunc('month', now());
		`, userID).Scan(&cardVol, &cardIncome, &cardExpense)

		var cashback float64
		pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(cashback_awarded),0) FROM positions
			WHERE user_id=$1 AND status='closed' AND closed_at >= date_trunc('month', now());
		`, userID).Scan(&cashback)

		vouchers, _ := bankcards_sql.SumVoucherClaimsThisMonth(ctx, pool, userID)

		var savingsInterest float64
		pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount),0) FROM savings_history
			WHERE user_id=$1 AND entry_type='interest' AND created_at >= date_trunc('month', now());
		`, userID).Scan(&savingsInterest)

		total := tradesVol + transfersVol + cardVol + cashback + vouchers
		if total <= 0 {
			total = 1
		}

		categories := []category{
			{Key: "trades", Label: "Trades", Amount: tradesVol},
			{Key: "transfers", Label: "Transfers", Amount: transfersVol},
			{Key: "card", Label: "Crypto Card", Amount: cardVol},
			{Key: "cashback", Label: "Cashback", Amount: cashback},
			{Key: "vouchers", Label: "Vouchers & Promo", Amount: vouchers},
		}

		totalIncome := tradesIncome + transfersIncome + cardIncome + cashback + vouchers + savingsInterest
		totalExpense := tradesExpense + transfersExpense + cardExpense

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summaryResponse{
			Categories: categories, TotalIncome: totalIncome, TotalExpense: totalExpense,
			SavingsInterestThisMonth: savingsInterest,
		})
	}
}
