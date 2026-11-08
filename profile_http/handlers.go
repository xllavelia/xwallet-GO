package profile_http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"xwallet-server/auth_http"
	"xwallet-server/profile_sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type service interface {
	Own(context.Context, string) (profile_sql.OwnState, error)
	Update(context.Context, string, profile_sql.UpdateInput) (profile_sql.OwnState, error)
	Equip(context.Context, string, profile_sql.EquipInput) (profile_sql.OwnState, error)
	Purchase(context.Context, string, string) (profile_sql.OwnState, error)
	View(context.Context, string, string) (profile_sql.PublicProfile, error)
	SetAvatar(context.Context, string, []byte, []byte) (profile_sql.OwnState, error)
	Community(context.Context, string) (profile_sql.CommunityState, error)
	Search(context.Context, string, string) ([]profile_sql.PublicProfile, error)
	Friend(context.Context, string, string, string) (profile_sql.Relationship, error)
}

type handlers struct{ store service }

func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool) {
	register(mux, profile_sql.NewStore(pool))
}

func register(mux *http.ServeMux, store service) {
	h := handlers{store: store}
	for path, handler := range map[string]http.HandlerFunc{
		"/profile": h.profile, "/profile/update": h.update,
		"/profile/equip": h.equip, "/profile/purchase": h.purchase,
		"/profile/view": h.view, "/profile/avatar": h.avatar,
		"/community": h.community, "/community/search": h.search,
		"/community/friend": h.friend,
	} {
		mux.HandleFunc(path, auth_http.WithCORS(auth_http.RequireAuth(handler)))
	}
}

func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(value)
}

func respondError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "could not complete request"
	switch {
	case errors.Is(err, profile_sql.ErrInvalid), errors.Is(err, ErrAvatarInvalid):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrAvatarTooLarge):
		status, message = http.StatusRequestEntityTooLarge, err.Error()
	case errors.Is(err, profile_sql.ErrUnauthorized):
		status, message = http.StatusUnauthorized, err.Error()
	case errors.Is(err, profile_sql.ErrForbidden), errors.Is(err, profile_sql.ErrNotOwned):
		status, message = http.StatusForbidden, err.Error()
	case errors.Is(err, profile_sql.ErrNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, profile_sql.ErrNotPurchasable), errors.Is(err, profile_sql.ErrInsufficientLavx),
		errors.Is(err, profile_sql.ErrUsernameTaken), errors.Is(err, profile_sql.ErrTransition):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, message = http.StatusServiceUnavailable, "request unavailable"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func playerFor(w http.ResponseWriter, r *http.Request, methods ...string) (string, bool) {
	allowed := false
	for _, method := range methods {
		if r.Method == method {
			allowed = true
		}
	}
	if !allowed {
		allow := ""
		for i, method := range methods {
			if i > 0 {
				allow += ", "
			}
			allow += method
		}
		w.Header().Set("Allow", allow)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", false
	}
	a, ok := auth_http.UserFromContext(r)
	if !ok || profile_sql.ValidatePlayerID(a.PlayerID) != nil {
		respondError(w, profile_sql.ErrUnauthorized)
		return "", false
	}
	return a.PlayerID, true // Имя и роль из JWT не определяют права доступа.
}

func decodeRequest(w http.ResponseWriter, r *http.Request, max int64, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			respondError(w, ErrAvatarTooLarge)
		} else {
			respondError(w, profile_sql.ErrInvalid)
		}
		return false
	}
	if !utf8.Valid(data) {
		respondError(w, profile_sql.ErrInvalid)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		respondError(w, profile_sql.ErrInvalid)
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		respondError(w, profile_sql.ErrInvalid)
		return false
	}
	return true
}

func (h handlers) profile(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodGet)
	if !ok {
		return
	}
	state, err := h.store.Own(r.Context(), player)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) update(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodPost)
	if !ok {
		return
	}
	var req struct {
		Username    *string `json:"username"`
		Bio         *string `json:"bio"`
		ShowBalance *bool   `json:"showBalance"`
	}
	if !decodeRequest(w, r, 8*1024, &req) {
		return
	}
	if req.Username == nil || req.Bio == nil || req.ShowBalance == nil {
		respondError(w, profile_sql.ErrInvalid)
		return
	}
	in := profile_sql.UpdateInput{Username: *req.Username, Bio: *req.Bio, ShowBalance: *req.ShowBalance}
	if err := profile_sql.ValidateUpdate(in); err != nil {
		respondError(w, err)
		return
	}
	state, err := h.store.Update(r.Context(), player, in)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) equip(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodPost)
	if !ok {
		return
	}
	var in profile_sql.EquipInput
	if !decodeRequest(w, r, 8*1024, &in) {
		return
	}
	if err := profile_sql.ValidateEquip(in); err != nil {
		respondError(w, err)
		return
	}
	state, err := h.store.Equip(r.Context(), player, in)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) purchase(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodPost)
	if !ok {
		return
	}
	var req struct {
		CosmeticID string `json:"cosmeticId"`
	}
	if !decodeRequest(w, r, 8*1024, &req) {
		return
	}
	c, exists := profile_sql.GetCosmetic(req.CosmeticID)
	if !exists {
		respondError(w, profile_sql.ErrInvalid)
		return
	}
	if c.AdminOnly || c.Source != "shop" || c.Price <= 0 {
		respondError(w, profile_sql.ErrNotPurchasable)
		return
	}
	state, err := h.store.Purchase(r.Context(), player, req.CosmeticID)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) view(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodGet)
	if !ok {
		return
	}
	target := r.URL.Query().Get("playerId")
	if err := profile_sql.ValidatePlayerID(target); err != nil {
		respondError(w, err)
		return
	}
	profile, err := h.store.View(r.Context(), player, target)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, profile)
}

func (h handlers) avatar(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodPost, http.MethodDelete)
	if !ok {
		return
	}
	var avatar, thumb []byte
	if r.Method == http.MethodPost {
		var req struct {
			Image string `json:"image"`
		}
		if !decodeRequest(w, r, AvatarMaxRequestBytes, &req) {
			return
		}
		var err error
		avatar, thumb, err = ProcessAvatar(req.Image)
		if err != nil {
			respondError(w, err)
			return
		}
	}
	state, err := h.store.SetAvatar(r.Context(), player, avatar, thumb)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) community(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodGet)
	if !ok {
		return
	}
	state, err := h.store.Community(r.Context(), player)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, state)
}

func (h handlers) search(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodGet)
	if !ok {
		return
	}
	query := r.URL.Query().Get("q")
	if err := profile_sql.ValidateSearch(query); err != nil {
		respondError(w, err)
		return
	}
	players, err := h.store.Search(r.Context(), player, query)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, players)
}

func (h handlers) friend(w http.ResponseWriter, r *http.Request) {
	player, ok := playerFor(w, r, http.MethodPost)
	if !ok {
		return
	}
	var req struct {
		PlayerID string `json:"playerId"`
		Action   string `json:"action"`
	}
	if !decodeRequest(w, r, 8*1024, &req) {
		return
	}
	if profile_sql.ValidatePlayerID(req.PlayerID) != nil {
		respondError(w, profile_sql.ErrInvalid)
		return
	}
	switch req.Action {
	case "request", "accept", "decline", "cancel", "remove":
	default:
		respondError(w, profile_sql.ErrInvalid)
		return
	}
	relationship, err := h.store.Friend(r.Context(), player, req.PlayerID, req.Action)
	if err != nil {
		respondError(w, err)
		return
	}
	respond(w, relationship)
}
