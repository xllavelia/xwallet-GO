package admin_http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"xwallet-server/battlepass_sql"
	"xwallet-server/nft_sql"
	"xwallet-server/rewards_sql"
	"xwallet-server/users_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// Ошибки бизнес-логики отдаются текстом, внутренние — общим сообщением с логом.
var clientErrors = []error{
	nft_sql.ErrNotFound, nft_sql.ErrNotListed, nft_sql.ErrSoldOut, nft_sql.ErrInvalidValue, nft_sql.ErrWalletMissing,
	rewards_sql.ErrInvalidTrack, battlepass_sql.BPErrInvalidLevel,
}

func writeAdminError(w http.ResponseWriter, op string, err error) {
	for _, known := range clientErrors {
		if errors.Is(err, known) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	log.Printf("admin %s: %v", op, err)
	http.Error(w, "could not "+op, http.StatusInternalServerError)
}

func NftOverviewHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, err := nft_sql.GetAdminOverview(r.Context(), pool)
		if err != nil {
			writeAdminError(w, "load nft overview", err)
			return
		}
		writeJSON(w, o)
	}
}

func NftPlayerHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := users_sql.GetInternalIDByPlayerID(r.Context(), pool, r.URL.Query().Get("playerId"))
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		p, err := nft_sql.GetAdminPlayer(r.Context(), pool, userID)
		if err != nil {
			writeAdminError(w, "load player nfts", err)
			return
		}
		writeJSON(w, p)
	}
}

type nftActionRequest struct {
	Action     string  `json:"action"`
	PlayerID   string  `json:"playerId"`
	ItemID     int64   `json:"itemId"`
	ListingID  int64   `json:"listingId"`
	TemplateID string  `json:"templateId"`
	PackID     string  `json:"packId"`
	Rarity     string  `json:"rarity"`
	Pack       string  `json:"pack"`
	Count      int     `json:"count"`
	Supply     int     `json:"supply"`
	BasePrice  float64 `json:"basePrice"`
	IncomeHour float64 `json:"incomeHour"`
	Price      float64 `json:"price"`
	Stock      int     `json:"stock"`
	Confirm    string  `json:"confirm"`
}

func NftActionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req nftActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		playerID := func() (int, bool) {
			id, err := users_sql.GetInternalIDByPlayerID(ctx, pool, req.PlayerID)
			if err != nil {
				http.Error(w, "user not found", http.StatusNotFound)
				return 0, false
			}
			return id, true
		}
		switch req.Action {
		case "grant":
			userID, ok := playerID()
			if !ok {
				return
			}
			res, err := nft_sql.AdminGrant(ctx, pool, userID, nft_sql.AdminGrantSpec{
				TemplateID: req.TemplateID, Rarity: req.Rarity, Pack: req.Pack, Count: req.Count})
			if err != nil {
				writeAdminError(w, "grant nft", err)
				return
			}
			writeJSON(w, res)
		case "delete":
			if err := nft_sql.AdminDeleteItem(ctx, pool, req.ItemID); err != nil {
				writeAdminError(w, "delete nft", err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		case "wipe-player":
			userID, ok := playerID()
			if !ok {
				return
			}
			n, err := nft_sql.AdminWipePlayer(ctx, pool, userID)
			if err != nil {
				writeAdminError(w, "wipe player nfts", err)
				return
			}
			writeJSON(w, map[string]int64{"deleted": n})
		case "unlist":
			if err := nft_sql.AdminUnlist(ctx, pool, req.ListingID); err != nil {
				writeAdminError(w, "remove listing", err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		case "unlist-all":
			n, err := nft_sql.AdminUnlistAll(ctx, pool)
			if err != nil {
				writeAdminError(w, "clear market", err)
				return
			}
			writeJSON(w, map[string]int64{"removed": n})
		case "template":
			if err := nft_sql.AdminUpdateTemplate(ctx, pool, req.TemplateID, req.Supply, req.BasePrice, req.IncomeHour); err != nil {
				writeAdminError(w, "update template", err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		case "pack":
			if err := nft_sql.AdminUpdatePack(ctx, pool, req.PackID, req.Price, req.Stock); err != nil {
				writeAdminError(w, "update pack", err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		case "reset-all":
			// Сервер требует подтверждение, чтобы случайный запрос не стёр всю NFT-экономику.
			if req.Confirm != "RESET" {
				http.Error(w, "confirmation required", http.StatusBadRequest)
				return
			}
			res, err := nft_sql.AdminResetAll(ctx, pool)
			if err != nil {
				writeAdminError(w, "reset nfts", err)
				return
			}
			writeJSON(w, res)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	}
}

type progressResponse struct {
	Daily      *rewards_sql.AdminState      `json:"daily"`
	Battlepass *battlepass_sql.BPAdminState `json:"battlepass"`
}

func ProgressHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, err := users_sql.GetInternalIDByPlayerID(ctx, pool, r.URL.Query().Get("playerId"))
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		var resp progressResponse
		if resp.Daily, err = rewards_sql.GetAdminState(ctx, pool, userID); err != nil {
			writeAdminError(w, "load daily rewards", err)
			return
		}
		if resp.Battlepass, err = battlepass_sql.BPAdminLoad(ctx, pool, userID); err != nil {
			writeAdminError(w, "load battle pass", err)
			return
		}
		writeJSON(w, resp)
	}
}

type progressActionRequest struct {
	Action   string `json:"action"`
	PlayerID string `json:"playerId"`
	Track    string `json:"track"`
	Level    int    `json:"level"`
}

func ProgressActionHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req progressActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		userID, err := users_sql.GetInternalIDByPlayerID(ctx, pool, req.PlayerID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		switch req.Action {
		case "daily-reset":
			err = rewards_sql.AdminReset(ctx, pool, userID, req.Track)
		case "daily-ready":
			err = rewards_sql.AdminSkipCooldown(ctx, pool, userID)
		case "bp-level":
			_, err = battlepass_sql.BPAdminSetLevel(ctx, pool, userID, req.Level)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		if err != nil {
			writeAdminError(w, "update progress", err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}
