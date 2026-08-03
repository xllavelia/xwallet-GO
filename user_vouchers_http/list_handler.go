package user_vouchers_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/user_vouchers_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type voucherResponse struct {
	ID              int     `json:"id"`
	VoucherType     string  `json:"voucherType"`
	Status          string  `json:"status"`
	LimitAmount     float64 `json:"limitAmount"`
	UsedAmount      float64 `json:"usedAmount"`
	DurationSeconds int     `json:"durationSeconds"`
	ActivatedAt     string  `json:"activatedAt"`
	CreditAmount    float64 `json:"creditAmount"`
	Source          string  `json:"source"`
	CreatedAt       string  `json:"createdAt"`
}

func toResponse(v user_vouchers_sql.UserVoucher) voucherResponse {
	r := voucherResponse{
		ID: v.ID, VoucherType: v.VoucherType, Status: v.Status,
		UsedAmount: v.UsedAmount, Source: v.Source,
		CreatedAt: v.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if v.LimitAmount != nil {
		r.LimitAmount = *v.LimitAmount
	}
	if v.DurationSeconds != nil {
		r.DurationSeconds = *v.DurationSeconds
	}
	if v.ActivatedAt != nil {
		r.ActivatedAt = v.ActivatedAt.Format("2006-01-02T15:04:05Z")
	}
	if v.CreditAmount != nil {
		r.CreditAmount = *v.CreditAmount
	}
	return r
}

func ListVouchersHandler(pool *pgxpool.Pool) http.HandlerFunc {
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

		vouchers, err := user_vouchers_sql.GetByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load vouchers", http.StatusInternalServerError)
			return
		}

		items := make([]voucherResponse, 0, len(vouchers))
		for _, v := range vouchers {
			items = append(items, toResponse(v))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}
