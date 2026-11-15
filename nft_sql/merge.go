package nft_sql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MergeResult struct {
	Mode         string     `json:"mode"`
	TemplateID   string     `json:"templateId"`
	Rarity       string     `json:"rarity"`
	TargetRarity string     `json:"targetRarity"`
	Consumed     []ItemView `json:"consumed"`
	Item         ItemView   `json:"item"`
}

// Merge сжигает две копии одного шаблона (pair) или три разных NFT одной редкости (trio)
// и выдаёт случайную NFT следующей редкости. Результат гарантирован, поэтому цель ровно на одну ступень выше.
func Merge(ctx context.Context, pool *pgxpool.Pool, userID int, templateID string, itemIDs []int64) (*MergeResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	mode := "pair"
	var ids []int64
	if len(itemIDs) > 0 {
		mode = "trio"
		ids, err = trioIDs(ctx, tx, userID, itemIDs)
	} else {
		ids, err = pairIDs(ctx, tx, userID, templateID)
	}
	if err != nil {
		return nil, err
	}
	consumed, err := itemsByIDs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	src := consumed[0]
	idx := rarityIndex(src.Rarity)
	if idx < 0 || idx+1 >= len(Rarities) {
		return nil, ErrMergeTop
	}
	if mode == "trio" {
		seen := make(map[string]bool, len(consumed))
		for _, it := range consumed {
			if it.Rarity != src.Rarity || seen[it.TemplateID] {
				return nil, ErrMergeTrio
			}
			seen[it.TemplateID] = true
		}
	}
	next := Rarities[idx+1]

	total := 0.0
	for _, it := range consumed {
		total += it.Value
	}
	for _, id := range ids {
		if err = settleTx(ctx, tx, userID, id); err != nil {
			return nil, err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nft_items WHERE id = ANY($1)`, ids); err != nil {
		return nil, err
	}
	targetID, err := pickTemplate(ctx, tx, next.Key, -1, false)
	if err != nil {
		return nil, err
	}
	newID, _, err := mintTx(ctx, tx, userID, targetID, "Merge")
	if err != nil {
		return nil, err
	}
	granted, err := ItemByID(ctx, tx, newID)
	if err != nil {
		return nil, err
	}
	// Слияние попадает в ту же историю, что и улучшения: шанс 1, броска нет.
	if _, err = tx.Exec(ctx, `
INSERT INTO nft_upgrades(user_id, source_template_id, source_serial, source_value,
                         target_template_id, target_rarity, target_value, chance, roll, won, item_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, 1, 0, true, $8)`,
		userID, src.TemplateID, src.Serial, round2(total), targetID, granted.Rarity, granted.BasePrice, newID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &MergeResult{Mode: mode, TemplateID: src.TemplateID, Rarity: src.Rarity,
		TargetRarity: granted.Rarity, Consumed: consumed, Item: *granted}, nil
}

// pairIDs берёт две самые ранние незалистованные копии шаблона.
func pairIDs(ctx context.Context, tx pgx.Tx, userID int, templateID string) ([]int64, error) {
	rows, err := tx.Query(ctx, `
SELECT i.id FROM nft_items i
WHERE i.owner_id = $1 AND i.template_id = $2
  AND NOT EXISTS (SELECT 1 FROM nft_listings l WHERE l.item_id = i.id)
ORDER BY i.serial
LIMIT 2
FOR UPDATE OF i`, userID, templateID)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) < 2 {
		return nil, ErrMergePair
	}
	return ids, nil
}

// trioIDs проверяет владение и отсутствие листинга, блокируя ровно три переданных NFT.
func trioIDs(ctx context.Context, tx pgx.Tx, userID int, itemIDs []int64) ([]int64, error) {
	if len(itemIDs) != 3 {
		return nil, ErrMergeTrio
	}
	seen := make(map[int64]bool, 3)
	for _, id := range itemIDs {
		if seen[id] {
			return nil, ErrMergeTrio
		}
		seen[id] = true
	}
	rows, err := tx.Query(ctx, `
SELECT i.id, i.owner_id, EXISTS(SELECT 1 FROM nft_listings l WHERE l.item_id = i.id)
FROM nft_items i
WHERE i.id = ANY($1)
ORDER BY i.id
FOR UPDATE OF i`, itemIDs)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var owner int
		var listed bool
		if err = rows.Scan(&id, &owner, &listed); err != nil {
			rows.Close()
			return nil, err
		}
		if owner != userID {
			rows.Close()
			return nil, ErrNotOwner
		}
		if listed {
			rows.Close()
			return nil, ErrListed
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) != 3 {
		return nil, ErrNotFound
	}
	return ids, nil
}
