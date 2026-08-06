package positions_http

import (
	"encoding/json"
	"net/http"

	"xwallet-server/auth_http"
	"xwallet-server/positions_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type positionListItem struct {
	ID                int      `json:"id"`
	TradeID           string   `json:"tradeId"`
	Coin              string   `json:"coin"`
	Type              string   `json:"type"`
	EntryPrice        float64  `json:"entryPrice"`
	ClosePrice        *float64 `json:"closePrice"`
	Leverage          int      `json:"leverage"`
	Amount            float64  `json:"amount"`
	Margin            float64  `json:"margin"`
	Fees              float64  `json:"fees"`
	FeesPaidByVoucher bool     `json:"feesPaidByVoucher"`
	LiqPrice          float64  `json:"liqPrice"`
	AutoClose         bool     `json:"autoClose"`
	AutoCloseTarget   *float64 `json:"autoCloseTarget"`
	Pnl               *float64 `json:"pnl"`
	PnlPercent        *float64 `json:"pnlPercent"`
	Status            string   `json:"status"`
	Result            *string  `json:"result"`
	OpenedAt          string   `json:"openedAt"`
	ClosedAt          *string  `json:"closedAt"`
	XpAwarded         int      `json:"xpAwarded"`
}

func toListItem(p positions_sql.Position) positionListItem {
	var closedAt *string
	if p.ClosedAt != nil {
		s := p.ClosedAt.Format("2006-01-02T15:04:05Z")
		closedAt = &s
	}
	return positionListItem{
		ID: p.ID, TradeID: p.TradeID, Coin: p.Coin, Type: p.Type, EntryPrice: p.EntryPrice,
		ClosePrice: p.ClosePrice, Leverage: p.Leverage, Amount: p.Amount, Margin: p.Margin,
		Fees: p.Fees, FeesPaidByVoucher: p.FeesPaidByVoucher, LiqPrice: p.LiqPrice,
		AutoClose: p.AutoClose, AutoCloseTarget: p.AutoCloseTarget, Pnl: p.Pnl,
		PnlPercent: p.PnlPercent, Status: p.Status, Result: p.Result,
		OpenedAt: p.OpenedAt.Format("2006-01-02T15:04:05Z"), ClosedAt: closedAt, XpAwarded: p.XpAwarded,
	}
}

func ListOpenPositionsHandler(pool *pgxpool.Pool) http.HandlerFunc {
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
		positions, err := positions_sql.GetOpenPositionsByUserID(r.Context(), pool, userID)
		if err != nil {
			http.Error(w, "could not load positions", http.StatusInternalServerError)
			return
		}
		items := make([]positionListItem, 0, len(positions))
		for _, p := range positions {
			items = append(items, toListItem(p))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}
