package nft_sql

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidValue = errors.New("invalid value")

const AdminSource = "Admin"

type AdminCollection struct {
	CollectionView
	Active bool `json:"active"`
}

type AdminListing struct {
	ID        int64     `json:"id"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"createdAt"`
	SellerID  string    `json:"sellerId"`
	Seller    string    `json:"seller"`
	Item      ItemView  `json:"item"`
}

type AdminTotals struct {
	Items    int     `json:"items"`
	Owners   int     `json:"owners"`
	Listings int     `json:"listings"`
	Sales    int     `json:"sales"`
	Volume   float64 `json:"volume"`
}

type AdminPackDef struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Caption string `json:"caption"`
}

type AdminOverview struct {
	Totals      AdminTotals       `json:"totals"`
	Collections []AdminCollection `json:"collections"`
	Listings    []AdminListing    `json:"listings"`
	Rarities    []string          `json:"rarities"`
	PackDefs    []AdminPackDef    `json:"packDefs"`
	NextDrop    UpcomingDrop      `json:"nextDrop"`
}

func GetAdminOverview(ctx context.Context, pool *pgxpool.Pool) (*AdminOverview, error) {
	if err := Ensure(ctx, pool); err != nil {
		return nil, err
	}
	o := &AdminOverview{Collections: []AdminCollection{}, Listings: []AdminListing{}, Rarities: []string{},
		PackDefs: []AdminPackDef{}, NextDrop: NextDrop(time.Now().UTC())}
	for _, r := range Rarities {
		o.Rarities = append(o.Rarities, r.Key)
	}
	for _, p := range packDefs {
		o.PackDefs = append(o.PackDefs, AdminPackDef{p.Code, p.Name, p.Caption})
	}
	if err := pool.QueryRow(ctx, `
SELECT (SELECT COUNT(*) FROM nft_items), (SELECT COUNT(DISTINCT owner_id) FROM nft_items),
       (SELECT COUNT(*) FROM nft_listings), (SELECT COUNT(*) FROM nft_sales WHERE kind <> 'instant'),
       (SELECT COALESCE(SUM(price), 0)::float8 FROM nft_sales WHERE kind <> 'instant')`).
		Scan(&o.Totals.Items, &o.Totals.Owners, &o.Totals.Listings, &o.Totals.Sales, &o.Totals.Volume); err != nil {
		return nil, err
	}

	index := map[int]int{}
	rows, err := pool.Query(ctx, `
SELECT id, name, theme, tagline, starts_at, ends_at, starts_at <= now() AND ends_at > now()
FROM nft_collections ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c AdminCollection
		if err = rows.Scan(&c.ID, &c.Name, &c.Theme, &c.Tagline, &c.StartsAt, &c.EndsAt, &c.Active); err != nil {
			rows.Close()
			return nil, err
		}
		c.Templates, c.Packs = []TemplateView{}, []PackView{}
		index[c.ID] = len(o.Collections)
		o.Collections = append(o.Collections, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `
SELECT collection_id, id, name, rarity, art, base_price::float8, income_hour::float8, supply, minted
FROM nft_templates ORDER BY collection_id, slot`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid int
		var t TemplateView
		if err = rows.Scan(&cid, &t.ID, &t.Name, &t.Rarity, &t.Art, &t.BasePrice, &t.IncomeHour, &t.Supply, &t.Minted); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := index[cid]; ok {
			o.Collections[i].Templates = append(o.Collections[i].Templates, t)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `
SELECT id, collection_id, code, name, kind, caption, slots, price::float8, stock, sold
FROM nft_packs ORDER BY collection_id, price`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PackView
		var slots []byte
		if err = rows.Scan(&p.ID, &p.CollectionID, &p.Code, &p.Name, &p.Kind, &p.Caption, &slots, &p.Price, &p.Stock, &p.Sold); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(slots, &p.Slots)
		if i, ok := index[p.CollectionID]; ok {
			o.Collections[i].Packs = append(o.Collections[i].Packs, p)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// LEFT JOIN: лоты удалённых игроков тоже видны админу, чтобы их можно было снять.
	rows, err = pool.Query(ctx, `SELECT `+itemCols+`, l.created_at, COALESCE(u.player_id, ''), COALESCE(u.username, 'deleted user')
FROM `+itemFrom+` LEFT JOIN users u ON u.id = l.seller_id
WHERE l.id IS NOT NULL ORDER BY l.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a AdminListing
		item, err := scanItem(rows, &a.CreatedAt, &a.SellerID, &a.Seller)
		if err != nil {
			return nil, err
		}
		a.ID, a.Price, a.Item = item.ListingID, item.ListPrice, item
		o.Listings = append(o.Listings, a)
	}
	return o, rows.Err()
}

type AdminPlayer struct {
	Items   []ItemView `json:"items"`
	Count   int        `json:"count"`
	Listed  int        `json:"listed"`
	Value   float64    `json:"value"`
	Pending float64    `json:"pending"`
	Earned  float64    `json:"earned"`
	Sold    float64    `json:"sold"`
	Spent   float64    `json:"spent"`
}

func GetAdminPlayer(ctx context.Context, pool *pgxpool.Pool, userID int) (*AdminPlayer, error) {
	st, err := GetState(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	p := st.Profile
	return &AdminPlayer{Items: st.Items, Count: p.Count, Listed: p.Listed, Value: p.Value,
		Pending: p.Pending, Earned: p.Earned, Sold: p.Sold, Spent: p.Spent}, nil
}

type AdminGrantSpec struct {
	TemplateID string `json:"templateId"`
	Rarity     string `json:"rarity"`
	Pack       string `json:"pack"`
	Count      int    `json:"count"`
}

type AdminGrantResult struct {
	Items []ItemView `json:"items"`
	Note  string     `json:"note,omitempty"`
}

// AdminGrant выдаёт конкретный шаблон, случайные NFT редкости или пак свежего дропа.
func AdminGrant(ctx context.Context, pool *pgxpool.Pool, userID int, spec AdminGrantSpec) (*AdminGrantResult, error) {
	if spec.Count < 1 || spec.Count > 50 {
		return nil, ErrInvalidValue
	}
	if err := Ensure(ctx, pool); err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	res := &AdminGrantResult{Items: []ItemView{}}
	switch {
	case spec.TemplateID != "":
		ids := make([]int64, 0, spec.Count)
		for n := 0; n < spec.Count; n++ {
			id, _, err := mintTx(ctx, tx, userID, spec.TemplateID, AdminSource)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if res.Items, err = itemsByIDs(ctx, tx, ids); err != nil {
			return nil, err
		}
	case spec.Pack != "":
		for n := 0; n < spec.Count; n++ {
			items, note, err := GrantTx(ctx, tx, userID, GrantSpec{Pack: spec.Pack}, AdminSource)
			if err != nil {
				return nil, err
			}
			res.Items = append(res.Items, items...)
			if note != "" {
				res.Note = note
			}
		}
	default:
		items, note, err := GrantTx(ctx, tx, userID, GrantSpec{Rarity: spec.Rarity, Count: spec.Count}, AdminSource)
		if err != nil {
			return nil, err
		}
		res.Items, res.Note = items, note
	}
	return res, tx.Commit(ctx)
}

// AdminDeleteItem сжигает экземпляр (лот удаляется каскадом); накопленный доход остаётся владельцу.
func AdminDeleteItem(ctx context.Context, pool *pgxpool.Pool, itemID int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner int
	err = tx.QueryRow(ctx, `SELECT owner_id FROM nft_items WHERE id = $1 FOR UPDATE`, itemID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = settleTx(ctx, tx, owner, itemID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_items WHERE id = $1`, itemID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdminWipePlayer удаляет все NFT игрока, его лоты и NFT-статистику.
func AdminWipePlayer(ctx context.Context, pool *pgxpool.Pool, userID int) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM nft_listings WHERE seller_id = $1`, userID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM nft_items WHERE owner_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_stats WHERE user_id = $1`, userID); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

func AdminUnlist(ctx context.Context, pool *pgxpool.Pool, listingID int64) error {
	tag, err := pool.Exec(ctx, `DELETE FROM nft_listings WHERE id = $1`, listingID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotListed
	}
	return nil
}

func AdminUnlistAll(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM nft_listings`)
	return tag.RowsAffected(), err
}

func validMoney(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 && v <= MaxListPrice
}

// AdminUpdateTemplate меняет тираж и экономику шаблона; тираж нельзя опустить ниже уже выпущенного.
func AdminUpdateTemplate(ctx context.Context, pool *pgxpool.Pool, id string, supply int, base, income float64) error {
	if supply < 1 || !validMoney(base) || math.IsNaN(income) || income < 0 || income > base {
		return ErrInvalidValue
	}
	tag, err := pool.Exec(ctx, `
UPDATE nft_templates SET supply = $2, base_price = $3, income_hour = $4 WHERE id = $1 AND minted <= $2`,
		id, supply, round2(base), income)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidValue
	}
	return nil
}

func AdminUpdatePack(ctx context.Context, pool *pgxpool.Pool, id string, price float64, stock int) error {
	if stock < 0 || !validMoney(price) {
		return ErrInvalidValue
	}
	tag, err := pool.Exec(ctx, `UPDATE nft_packs SET price = $2, stock = $3 WHERE id = $1 AND sold <= $3`, id, round2(price), stock)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidValue
	}
	return nil
}

type ResetResult struct {
	Items       int64 `json:"items"`
	Collections int   `json:"collections"`
}

// AdminResetAll стирает все NFT (включая выданные из BP и ежедневных) и пересоздаёт дропы в исходном виде.
func AdminResetAll(ctx context.Context, pool *pgxpool.Pool) (*ResetResult, error) {
	ensureMu.Lock()
	defer ensureMu.Unlock()
	now := time.Now().UTC()
	k := dropIndex(now)
	day := now.Format("2006-01-02")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1735390078)`); err != nil {
		return nil, err
	}
	res := &ResetResult{}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM nft_items`).Scan(&res.Items); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
TRUNCATE nft_listings, nft_items, nft_sales, nft_stats, nft_game_listings, nft_packs, nft_templates, nft_collections
RESTART IDENTITY`); err != nil {
		return nil, err
	}
	for j := max(0, k-2); j <= k; j++ {
		if err = insertCollection(ctx, tx, generate(j)); err != nil {
			return nil, err
		}
		res.Collections++
	}
	if err = insertGameListings(ctx, tx, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		// Кэш сбрасывается, чтобы следующий Ensure перепроверил базу после неудачи.
		ensuredDrop, ensuredDay = -1, ""
		return nil, err
	}
	ensuredDrop, ensuredDay = k, day
	return res, nil
}
