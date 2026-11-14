package nft_sql

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type scanner interface {
	Scan(dest ...any) error
}

type CollectionRef struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Theme string `json:"theme"`
}

type ItemView struct {
	ID           int64           `json:"id"`
	TemplateID   string          `json:"templateId"`
	Name         string          `json:"name"`
	Serial       int             `json:"serial"`
	Supply       int             `json:"supply"`
	Minted       int             `json:"minted"`
	Rarity       string          `json:"rarity"`
	Art          json.RawMessage `json:"art"`
	Collection   CollectionRef   `json:"collection"`
	BasePrice    float64         `json:"basePrice"`
	Value        float64         `json:"value"`
	InstantPrice float64         `json:"instantPrice"`
	IncomeHour   float64         `json:"incomeHour"`
	Source       string          `json:"source"`
	MintedAt     time.Time       `json:"mintedAt"`
	ListingID    int64           `json:"listingId,omitempty"`
	ListPrice    float64         `json:"listPrice,omitempty"`
}

const itemCols = `i.id, t.id, t.name, i.serial, t.supply, t.minted, t.rarity, t.art, c.id, c.name, c.theme,
t.base_price::float8, t.income_hour::float8, i.source, i.minted_at, COALESCE(l.id, 0), COALESCE(l.price, 0)::float8`

const itemFrom = `nft_items i
JOIN nft_templates t ON t.id = i.template_id
JOIN nft_collections c ON c.id = t.collection_id
LEFT JOIN nft_listings l ON l.item_id = i.id`

func scanItem(row scanner, extra ...any) (ItemView, error) {
	var v ItemView
	dest := []any{&v.ID, &v.TemplateID, &v.Name, &v.Serial, &v.Supply, &v.Minted, &v.Rarity, &v.Art,
		&v.Collection.ID, &v.Collection.Name, &v.Collection.Theme, &v.BasePrice, &v.IncomeHour,
		&v.Source, &v.MintedAt, &v.ListingID, &v.ListPrice}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return v, err
	}
	v.Value = ItemValue(v.BasePrice, v.Serial)
	v.InstantPrice = InstantPrice(v.Value)
	return v, nil
}

func itemsByIDs(ctx context.Context, q querier, ids []int64) ([]ItemView, error) {
	items := make([]ItemView, 0, len(ids))
	if len(ids) == 0 {
		return items, nil
	}
	rows, err := q.Query(ctx, `SELECT `+itemCols+` FROM `+itemFrom+` WHERE i.id = ANY($1) ORDER BY i.id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func ItemByID(ctx context.Context, q querier, id int64) (*ItemView, error) {
	items, err := itemsByIDs(ctx, q, []int64{id})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

func creditTx(ctx context.Context, tx pgx.Tx, userID int, amount float64) error {
	tag, err := tx.Exec(ctx, `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2`, amount, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrWalletMissing
	}
	return nil
}

func debitTx(ctx context.Context, tx pgx.Tx, userID int, amount float64) error {
	tag, err := tx.Exec(ctx, `
UPDATE wallets SET balance = balance - $1, updated_at = now() WHERE user_id = $2 AND balance >= $1`, amount, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInsufficientFunds
	}
	return nil
}

func addStatTx(ctx context.Context, tx pgx.Tx, userID int, sold, spent float64) error {
	_, err := tx.Exec(ctx, `
INSERT INTO nft_stats(user_id, sold, spent) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET sold = nft_stats.sold + EXCLUDED.sold, spent = nft_stats.spent + EXCLUDED.spent`,
		userID, sold, spent)
	return err
}

// settleTx переносит накопленный пассивный доход в точный остаток carry; itemID = 0 — все NFT владельца.
func settleTx(ctx context.Context, tx pgx.Tx, userID int, itemID int64) error {
	var amount float64
	if err := tx.QueryRow(ctx, `
WITH old AS (
	SELECT i.id, t.income_hour * GREATEST(0, EXTRACT(EPOCH FROM (now() - i.accrued_at))) / 3600 AS amt
	FROM nft_items i JOIN nft_templates t ON t.id = i.template_id
	WHERE i.owner_id = $1 AND ($2::bigint = 0 OR i.id = $2)
	FOR UPDATE OF i
), upd AS (
	UPDATE nft_items SET accrued_at = now() WHERE id IN (SELECT id FROM old)
)
SELECT COALESCE(SUM(amt), 0)::float8 FROM old`, userID, itemID).Scan(&amount); err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
INSERT INTO nft_stats(user_id, carry) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET carry = nft_stats.carry + EXCLUDED.carry`, userID, amount)
	return err
}

type CollectResult struct {
	Collected float64 `json:"collected"`
	Pending   float64 `json:"pending"`
}

// Collect выплачивает целые центы, дробный остаток копится дальше.
func Collect(ctx context.Context, pool *pgxpool.Pool, userID int) (*CollectResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = settleTx(ctx, tx, userID, 0); err != nil {
		return nil, err
	}
	var carry float64
	err = tx.QueryRow(ctx, `SELECT carry::float8 FROM nft_stats WHERE user_id = $1 FOR UPDATE`, userID).Scan(&carry)
	if errors.Is(err, pgx.ErrNoRows) {
		return &CollectResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	payout := math.Floor(carry*100) / 100
	if payout > 0 {
		if err = creditTx(ctx, tx, userID, payout); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
UPDATE nft_stats SET carry = carry - $2, earned = earned + $2 WHERE user_id = $1`, userID, payout); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CollectResult{Collected: payout, Pending: carry - payout}, nil
}

func mintTx(ctx context.Context, tx pgx.Tx, userID int, templateID, source string) (int64, int, error) {
	var serial int
	err := tx.QueryRow(ctx, `
UPDATE nft_templates SET minted = minted + 1 WHERE id = $1 AND minted < supply RETURNING minted`, templateID).Scan(&serial)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrSoldOut
	}
	if err != nil {
		return 0, 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `
INSERT INTO nft_items(template_id, serial, owner_id, source) VALUES ($1, $2, $3, $4) RETURNING id`,
		templateID, serial, userID, source).Scan(&id)
	return id, serial, err
}

// pickTemplate выбирает шаблон редкости с остатком тиража; collectionID < 0 — любая коллекция.
func pickTemplate(ctx context.Context, tx pgx.Tx, rarity string, collectionID int, activeOnly bool) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
SELECT t.id FROM nft_templates t JOIN nft_collections c ON c.id = t.collection_id
WHERE t.rarity = $1 AND t.minted < t.supply
  AND ($2::int < 0 OR t.collection_id = $2)
  AND (NOT $3 OR (c.starts_at <= now() AND c.ends_at > now()))
ORDER BY random() LIMIT 1`, rarity, collectionID, activeOnly).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSoldOut
	}
	return id, err
}

func rollRarity() string {
	n := rand.IntN(1000)
	for _, r := range Rarities {
		if n < r.Odds {
			return r.Key
		}
		n -= r.Odds
	}
	return Rarities[0].Key
}

func lowerRarity(key string) string {
	for i, r := range Rarities {
		if r.Key == key && i > 0 {
			return Rarities[i-1].Key
		}
	}
	return ""
}

// mintSlotsTx минтит содержимое пака; strict запрещает подмену коллекции, мистери-капсула понижает редкость.
func mintSlotsTx(ctx context.Context, tx pgx.Tx, userID, collectionID int, kind string, slots []packSlot, source string, strict bool) ([]int64, float64, error) {
	var ids []int64
	cash := 0.0
	for _, s := range slots {
		for n := 0; n < s.Count; n++ {
			rarity := s.Rarity
			if kind == "mystery" {
				rarity = rollRarity()
			}
			var tid string
			var err error
			for rarity != "" {
				tid, err = pickTemplate(ctx, tx, rarity, collectionID, collectionID < 0)
				if err == nil || !errors.Is(err, ErrSoldOut) || kind != "mystery" {
					break
				}
				rarity = lowerRarity(rarity)
			}
			if errors.Is(err, ErrSoldOut) && !strict {
				tid, err = pickTemplate(ctx, tx, s.Rarity, -1, true)
				if errors.Is(err, ErrSoldOut) {
					tid, err = pickTemplate(ctx, tx, s.Rarity, -1, false)
				}
				if errors.Is(err, ErrSoldOut) {
					// Весь тираж редкости разобран — награда не теряется, а выплачивается деньгами.
					rc, _ := rarityOf(s.Rarity)
					cash += round2((rc.Lo + rc.Hi) / 2 * InstantRate)
					continue
				}
			}
			if err != nil {
				return nil, 0, err
			}
			id, _, err := mintTx(ctx, tx, userID, tid, source)
			if err != nil {
				return nil, 0, err
			}
			ids = append(ids, id)
		}
	}
	if cash > 0 {
		if err := creditTx(ctx, tx, userID, cash); err != nil {
			return nil, 0, err
		}
	}
	return ids, cash, nil
}

type GrantSpec struct {
	Rarity string
	Count  int
	Pack   string
}

func latestCollectionTx(ctx context.Context, tx pgx.Tx) (int, error) {
	var id int
	err := tx.QueryRow(ctx, `
SELECT id FROM nft_collections WHERE starts_at <= now() AND ends_at > now() ORDER BY id DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrSoldOut
	}
	return id, err
}

// GrantTx выдаёт NFT из наград (Battle Pass, ежедневные). Паки берутся из самого свежего дропа.
func GrantTx(ctx context.Context, tx pgx.Tx, userID int, spec GrantSpec, source string) ([]ItemView, string, error) {
	var ids []int64
	var cash float64
	var err error
	if spec.Pack != "" {
		def, ok := packDefByCode(spec.Pack)
		if !ok {
			return nil, "", ErrNotFound
		}
		collectionID, err := latestCollectionTx(ctx, tx)
		if err != nil {
			return nil, "", err
		}
		ids, cash, err = mintSlotsTx(ctx, tx, userID, collectionID, def.Kind, def.Slots, source+" · "+def.Name, false)
		if err != nil {
			return nil, "", err
		}
	} else {
		if !ValidRarity(spec.Rarity) {
			return nil, "", ErrNotFound
		}
		count := max(spec.Count, 1)
		ids, cash, err = mintSlotsTx(ctx, tx, userID, -1, "random", []packSlot{{spec.Rarity, count}}, source, false)
		if err != nil {
			return nil, "", err
		}
	}
	items, err := itemsByIDs(ctx, tx, ids)
	if err != nil {
		return nil, "", err
	}
	note := ""
	if cash > 0 {
		note = "NFT supply sold out — $" + itoa(int(cash)) + " added as cash"
	}
	return items, note, nil
}

type SellResult struct {
	ItemID int64   `json:"itemId"`
	Price  float64 `json:"price"`
}

// InstantSell продаёт NFT системе за 80% оценки; экземпляр сжигается, тираж не восстанавливается.
func InstantSell(ctx context.Context, pool *pgxpool.Pool, userID int, itemID int64) (*SellResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var owner, serial int
	var base float64
	var templateID string
	var listed bool
	err = tx.QueryRow(ctx, `
SELECT i.owner_id, i.serial, i.template_id, t.base_price::float8,
       EXISTS(SELECT 1 FROM nft_listings l WHERE l.item_id = i.id)
FROM nft_items i JOIN nft_templates t ON t.id = i.template_id
WHERE i.id = $1 FOR UPDATE OF i`, itemID).Scan(&owner, &serial, &templateID, &base, &listed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, ErrNotOwner
	}
	if listed {
		return nil, ErrListed
	}
	if err = settleTx(ctx, tx, userID, itemID); err != nil {
		return nil, err
	}
	price := InstantPrice(ItemValue(base, serial))
	if err = creditTx(ctx, tx, userID, price); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_items WHERE id = $1`, itemID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_sales(template_id, serial, seller_id, kind, price) VALUES ($1, $2, $3, 'instant', $4)`,
		templateID, serial, userID, price); err != nil {
		return nil, err
	}
	if err = addStatTx(ctx, tx, userID, price, 0); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &SellResult{ItemID: itemID, Price: price}, nil
}
