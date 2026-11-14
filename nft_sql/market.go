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

var ErrPriceChanged = errors.New("price changed, please refresh")

type Profile struct {
	Count      int            `json:"count"`
	Listed     int            `json:"listed"`
	Value      float64        `json:"value"`
	IncomeHour float64        `json:"incomeHour"`
	Pending    float64        `json:"pending"`
	Earned     float64        `json:"earned"`
	Sold       float64        `json:"sold"`
	Spent      float64        `json:"spent"`
	ByRarity   map[string]int `json:"byRarity"`
}

type TemplateView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Rarity     string          `json:"rarity"`
	Art        json.RawMessage `json:"art"`
	BasePrice  float64         `json:"basePrice"`
	IncomeHour float64         `json:"incomeHour"`
	Supply     int             `json:"supply"`
	Minted     int             `json:"minted"`
}

type PackView struct {
	ID           string     `json:"id"`
	CollectionID int        `json:"collectionId"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Caption      string     `json:"caption"`
	Slots        []packSlot `json:"slots"`
	Price        float64    `json:"price"`
	Stock        int        `json:"stock"`
	Sold         int        `json:"sold"`
}

type CollectionView struct {
	ID        int            `json:"id"`
	Name      string         `json:"name"`
	Theme     string         `json:"theme"`
	Tagline   string         `json:"tagline"`
	StartsAt  time.Time      `json:"startsAt"`
	EndsAt    time.Time      `json:"endsAt"`
	Templates []TemplateView `json:"templates"`
	Packs     []PackView     `json:"packs"`
}

type RarityView struct {
	Key    string  `json:"key"`
	Lo     float64 `json:"lo"`
	Hi     float64 `json:"hi"`
	Yield  float64 `json:"yield"`
	Supply int     `json:"supply"`
	Odds   int     `json:"odds"`
}

type ConfigView struct {
	InstantRate  float64      `json:"instantRate"`
	MarketFee    float64      `json:"marketFee"`
	MinListPrice float64      `json:"minListPrice"`
	MaxListPrice float64      `json:"maxListPrice"`
	Rarities     []RarityView `json:"rarities"`
}

type State struct {
	ServerTime  time.Time        `json:"serverTime"`
	Profile     Profile          `json:"profile"`
	Items       []ItemView       `json:"items"`
	Collections []CollectionView `json:"collections"`
	NextDrop    UpcomingDrop     `json:"nextDrop"`
	Config      ConfigView       `json:"config"`
}

func configView() ConfigView {
	out := ConfigView{InstantRate: InstantRate, MarketFee: MarketFee, MinListPrice: MinListPrice, MaxListPrice: MaxListPrice}
	for _, r := range Rarities {
		out.Rarities = append(out.Rarities, RarityView{r.Key, r.Lo, r.Hi, r.Yield, r.Supply, r.Odds})
	}
	return out
}

func GetState(ctx context.Context, pool *pgxpool.Pool, userID int) (*State, error) {
	now := time.Now().UTC()
	st := &State{ServerTime: now, Items: []ItemView{}, Collections: []CollectionView{}, NextDrop: NextDrop(now), Config: configView()}
	st.Profile.ByRarity = map[string]int{}

	rows, err := pool.Query(ctx, `SELECT `+itemCols+`,
t.income_hour::float8 * GREATEST(0, EXTRACT(EPOCH FROM (now() - i.accrued_at)))::float8 / 3600
FROM `+itemFrom+` WHERE i.owner_id = $1 ORDER BY t.base_price DESC, i.id`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var live float64
		v, err := scanItem(rows, &live)
		if err != nil {
			rows.Close()
			return nil, err
		}
		st.Items = append(st.Items, v)
		st.Profile.Count++
		st.Profile.Value += v.Value
		st.Profile.IncomeHour += v.IncomeHour
		st.Profile.Pending += live
		st.Profile.ByRarity[v.Rarity]++
		if v.ListingID > 0 {
			st.Profile.Listed++
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var carry float64
	err = pool.QueryRow(ctx, `
SELECT carry::float8, earned::float8, sold::float8, spent::float8 FROM nft_stats WHERE user_id = $1`, userID).
		Scan(&carry, &st.Profile.Earned, &st.Profile.Sold, &st.Profile.Spent)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	st.Profile.Pending += carry
	st.Profile.Value = round2(st.Profile.Value)

	cols, err := activeCollections(ctx, pool)
	if err != nil {
		return nil, err
	}
	st.Collections = cols
	return st, nil
}

func activeCollections(ctx context.Context, pool *pgxpool.Pool) ([]CollectionView, error) {
	out := []CollectionView{}
	index := map[int]int{}
	rows, err := pool.Query(ctx, `
SELECT id, name, theme, tagline, starts_at, ends_at FROM nft_collections
WHERE starts_at <= now() AND ends_at > now() ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c CollectionView
		if err = rows.Scan(&c.ID, &c.Name, &c.Theme, &c.Tagline, &c.StartsAt, &c.EndsAt); err != nil {
			rows.Close()
			return nil, err
		}
		c.Templates, c.Packs = []TemplateView{}, []PackView{}
		index[c.ID] = len(out)
		out = append(out, c)
	}
	rows.Close()
	if len(out) == 0 {
		return out, rows.Err()
	}
	ids := make([]int, 0, len(out))
	for _, c := range out {
		ids = append(ids, c.ID)
	}
	rows, err = pool.Query(ctx, `
SELECT collection_id, id, name, rarity, art, base_price::float8, income_hour::float8, supply, minted
FROM nft_templates WHERE collection_id = ANY($1) ORDER BY slot`, ids)
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
		out[index[cid]].Templates = append(out[index[cid]].Templates, t)
	}
	rows.Close()
	rows, err = pool.Query(ctx, `
SELECT id, collection_id, code, name, kind, caption, slots, price::float8, stock, sold
FROM nft_packs WHERE collection_id = ANY($1) ORDER BY price`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p PackView
		var slots []byte
		if err = rows.Scan(&p.ID, &p.CollectionID, &p.Code, &p.Name, &p.Kind, &p.Caption, &slots, &p.Price, &p.Stock, &p.Sold); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(slots, &p.Slots)
		out[index[p.CollectionID]].Packs = append(out[index[p.CollectionID]].Packs, p)
	}
	return out, rows.Err()
}

type ListingView struct {
	ID        int64     `json:"id"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"createdAt"`
	Seller    string    `json:"seller"`
	Mine      bool      `json:"mine"`
	Item      ItemView  `json:"item"`
}

type GameListingView struct {
	TemplateID string          `json:"templateId"`
	Name       string          `json:"name"`
	Rarity     string          `json:"rarity"`
	Art        json.RawMessage `json:"art"`
	Collection CollectionRef   `json:"collection"`
	BasePrice  float64         `json:"basePrice"`
	IncomeHour float64         `json:"incomeHour"`
	Supply     int             `json:"supply"`
	Minted     int             `json:"minted"`
	NextSerial int             `json:"nextSerial"`
	Value      float64         `json:"value"`
	Price      float64         `json:"price"`
	Left       int             `json:"left"`
}

type SaleView struct {
	Name      string          `json:"name"`
	Rarity    string          `json:"rarity"`
	Art       json.RawMessage `json:"art"`
	Serial    int             `json:"serial"`
	Kind      string          `json:"kind"`
	Price     float64         `json:"price"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Market struct {
	Listings  []ListingView     `json:"listings"`
	Game      []GameListingView `json:"game"`
	Recent    []SaleView        `json:"recent"`
	RefreshAt time.Time         `json:"refreshAt"`
}

func gamePrice(base float64, serial int, mult float64) float64 {
	return round2(ItemValue(base, serial) * mult)
}

func GetMarket(ctx context.Context, pool *pgxpool.Pool, userID int) (*Market, error) {
	now := time.Now().UTC()
	m := &Market{Listings: []ListingView{}, Game: []GameListingView{}, Recent: []SaleView{},
		RefreshAt: now.Truncate(24 * time.Hour).Add(24 * time.Hour)}

	rows, err := pool.Query(ctx, `SELECT `+itemCols+`, l.created_at, l.seller_id, u.username
FROM `+itemFrom+` JOIN users u ON u.id = l.seller_id
WHERE l.id IS NOT NULL ORDER BY l.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var lv ListingView
		var seller int
		item, err := scanItem(rows, &lv.CreatedAt, &seller, &lv.Seller)
		if err != nil {
			rows.Close()
			return nil, err
		}
		lv.ID, lv.Price, lv.Mine, lv.Item = item.ListingID, item.ListPrice, seller == userID, item
		m.Listings = append(m.Listings, lv)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `
SELECT t.id, t.name, t.rarity, t.art, c.id, c.name, c.theme, t.base_price::float8, t.income_hour::float8,
       t.supply, t.minted, g.mult::float8, g.stock - g.sold
FROM nft_game_listings g
JOIN nft_templates t ON t.id = g.template_id
JOIN nft_collections c ON c.id = t.collection_id
WHERE g.day = $1::date AND g.sold < g.stock AND t.minted < t.supply AND c.ends_at > now()
ORDER BY t.base_price`, now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g GameListingView
		var mult float64
		if err = rows.Scan(&g.TemplateID, &g.Name, &g.Rarity, &g.Art, &g.Collection.ID, &g.Collection.Name, &g.Collection.Theme,
			&g.BasePrice, &g.IncomeHour, &g.Supply, &g.Minted, &mult, &g.Left); err != nil {
			rows.Close()
			return nil, err
		}
		g.NextSerial = g.Minted + 1
		g.Value = ItemValue(g.BasePrice, g.NextSerial)
		g.Price = gamePrice(g.BasePrice, g.NextSerial, mult)
		g.Left = min(g.Left, g.Supply-g.Minted)
		m.Game = append(m.Game, g)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `
SELECT t.name, t.rarity, t.art, s.serial, s.kind, s.price::float8, s.created_at
FROM nft_sales s JOIN nft_templates t ON t.id = s.template_id
WHERE s.kind <> 'instant' ORDER BY s.created_at DESC LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s SaleView
		if err = rows.Scan(&s.Name, &s.Rarity, &s.Art, &s.Serial, &s.Kind, &s.Price, &s.CreatedAt); err != nil {
			return nil, err
		}
		m.Recent = append(m.Recent, s)
	}
	return m, rows.Err()
}

func lockOwnedTx(ctx context.Context, tx pgx.Tx, userID int, itemID int64) error {
	var owner int
	err := tx.QueryRow(ctx, `SELECT owner_id FROM nft_items WHERE id = $1 FOR UPDATE`, itemID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner != userID {
		return ErrNotOwner
	}
	return nil
}

// List выставляет NFT по любой цене; повторный вызов меняет цену существующего лота.
func List(ctx context.Context, pool *pgxpool.Pool, userID int, itemID int64, price float64) (*ItemView, error) {
	price = round2(price)
	if math.IsNaN(price) || price < MinListPrice || price > MaxListPrice {
		return nil, ErrInvalidPrice
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = lockOwnedTx(ctx, tx, userID, itemID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_listings(item_id, seller_id, price) VALUES ($1, $2, $3)
ON CONFLICT (item_id) DO UPDATE SET price = EXCLUDED.price`, itemID, userID, price); err != nil {
		return nil, err
	}
	item, err := ItemByID(ctx, tx, itemID)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit(ctx)
}

func Unlist(ctx context.Context, pool *pgxpool.Pool, userID int, itemID int64) error {
	tag, err := pool.Exec(ctx, `DELETE FROM nft_listings WHERE item_id = $1 AND seller_id = $2`, itemID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotListed
	}
	return nil
}

// BuyListing: покупатель платит цену лота, продавец получает её за вычетом 5% комиссии.
func BuyListing(ctx context.Context, pool *pgxpool.Pool, userID int, listingID int64) (*ItemView, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var itemID int64
	var seller int
	var price float64
	err = tx.QueryRow(ctx, `
SELECT item_id, seller_id, price::float8 FROM nft_listings WHERE id = $1 FOR UPDATE`, listingID).Scan(&itemID, &seller, &price)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrListingGone
	}
	if err != nil {
		return nil, err
	}
	if seller == userID {
		return nil, ErrOwnListing
	}
	if err = lockOwnedTx(ctx, tx, seller, itemID); err != nil {
		return nil, ErrListingGone
	}
	if err = debitTx(ctx, tx, userID, price); err != nil {
		return nil, err
	}
	if err = settleTx(ctx, tx, seller, itemID); err != nil {
		return nil, err
	}
	net := math.Floor(price*(1-MarketFee)*100) / 100
	if err = creditTx(ctx, tx, seller, net); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_listings WHERE id = $1`, listingID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
UPDATE nft_items SET owner_id = $2, accrued_at = now(), source = 'Market' WHERE id = $1`, itemID, userID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_sales(template_id, serial, seller_id, buyer_id, kind, price)
SELECT template_id, serial, $2, $3, 'market', $4 FROM nft_items WHERE id = $1`, itemID, seller, userID, price); err != nil {
		return nil, err
	}
	if err = addStatTx(ctx, tx, seller, net, 0); err != nil {
		return nil, err
	}
	if err = addStatTx(ctx, tx, userID, 0, price); err != nil {
		return nil, err
	}
	item, err := ItemByID(ctx, tx, itemID)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit(ctx)
}

// BuyGame минтит следующий номер с витрины xwallet; maxPrice защищает от сдвига цены.
func BuyGame(ctx context.Context, pool *pgxpool.Pool, userID int, templateID string, maxPrice float64) (*ItemView, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	day := time.Now().UTC().Format("2006-01-02")
	var mult, base float64
	var left int
	err = tx.QueryRow(ctx, `
SELECT g.mult::float8, g.stock - g.sold, t.base_price::float8
FROM nft_game_listings g
JOIN nft_templates t ON t.id = g.template_id
JOIN nft_collections c ON c.id = t.collection_id
WHERE g.day = $1::date AND g.template_id = $2 AND c.ends_at > now()
FOR UPDATE OF g`, day, templateID).Scan(&mult, &left, &base)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrListingGone
	}
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, ErrSoldOut
	}
	id, serial, err := mintTx(ctx, tx, userID, templateID, "xwallet Market")
	if err != nil {
		return nil, err
	}
	price := gamePrice(base, serial, mult)
	if maxPrice > 0 && price > maxPrice+0.001 {
		return nil, ErrPriceChanged
	}
	if err = debitTx(ctx, tx, userID, price); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
UPDATE nft_game_listings SET sold = sold + 1 WHERE day = $1::date AND template_id = $2`, day, templateID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_sales(template_id, serial, buyer_id, kind, price) VALUES ($1, $2, $3, 'game', $4)`,
		templateID, serial, userID, price); err != nil {
		return nil, err
	}
	if err = addStatTx(ctx, tx, userID, 0, price); err != nil {
		return nil, err
	}
	item, err := ItemByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit(ctx)
}

type PackResult struct {
	Pack  PackView   `json:"pack"`
	Items []ItemView `json:"items"`
}

func BuyPack(ctx context.Context, pool *pgxpool.Pool, userID int, packID string) (*PackResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var p PackView
	var slots []byte
	var active bool
	err = tx.QueryRow(ctx, `
SELECT p.id, p.collection_id, p.code, p.name, p.kind, p.caption, p.slots, p.price::float8, p.stock, p.sold,
       c.starts_at <= now() AND c.ends_at > now()
FROM nft_packs p JOIN nft_collections c ON c.id = p.collection_id
WHERE p.id = $1 FOR UPDATE OF p`, packID).
		Scan(&p.ID, &p.CollectionID, &p.Code, &p.Name, &p.Kind, &p.Caption, &slots, &p.Price, &p.Stock, &p.Sold, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrDropClosed
	}
	if p.Sold >= p.Stock {
		return nil, ErrSoldOut
	}
	_ = json.Unmarshal(slots, &p.Slots)
	if err = debitTx(ctx, tx, userID, p.Price); err != nil {
		return nil, err
	}
	ids, _, err := mintSlotsTx(ctx, tx, userID, p.CollectionID, p.Kind, p.Slots, "Pack · "+p.Name, true)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE nft_packs SET sold = sold + 1 WHERE id = $1`, p.ID); err != nil {
		return nil, err
	}
	if err = addStatTx(ctx, tx, userID, 0, p.Price); err != nil {
		return nil, err
	}
	items, err := itemsByIDs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	p.Sold++
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PackResult{Pack: p, Items: items}, nil
}
