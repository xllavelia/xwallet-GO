package nft_sql

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type genTemplate struct {
	ID     string
	Slot   int
	Name   string
	Rarity string
	Art    map[string]any
	Base   float64
	Income float64
	Supply int
}

type genPack struct {
	ID      string
	Code    string
	Name    string
	Kind    string
	Caption string
	Slots   []packSlot
	Price   float64
	Stock   int
}

type genCollection struct {
	ID        int
	Name      string
	Theme     string
	Tagline   string
	StartsAt  time.Time
	EndsAt    time.Time
	Templates []genTemplate
	Packs     []genPack
}

func dropIndex(now time.Time) int {
	if now.Before(Epoch) {
		return 0
	}
	return int(now.Sub(Epoch) / CollectionEvery)
}

// Шаг 3 взаимно прост с 8 темами — соседние дропы никогда не повторяют тему.
func dropTheme(k int) theme {
	return themes[(k*3)%len(themes)]
}

func dropName(k int) string {
	t := dropTheme(k)
	return t.Names[(k/len(themes))%len(t.Names)]
}

func dropStart(k int) time.Time {
	return Epoch.Add(time.Duration(k) * CollectionEvery)
}

func nicePrice(v float64) float64 {
	if v < 1 {
		return round2(v)
	}
	return math.Ceil(v) - 0.01
}

// generate детерминированно строит коллекцию по номеру дропа — сервер и повторные запуски получают одно и то же.
func generate(k int) genCollection {
	rng := rand.New(rand.NewPCG(uint64(k)+1, 0x9e3779b97f4a7c15))
	t := dropTheme(k)
	sprites := append([]string(nil), t.Sprites...)
	rng.Shuffle(len(sprites), func(i, j int) { sprites[i], sprites[j] = sprites[j], sprites[i] })
	words := append([]string(nil), t.Words...)
	rng.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })

	c := genCollection{
		ID: k, Name: dropName(k), Theme: t.Key, Tagline: t.Tagline,
		StartsAt: dropStart(k), EndsAt: dropStart(k).Add(CollectionLife),
	}
	sums := map[string]float64{}
	counts := map[string]int{}
	for slot, rar := range dropSlots {
		rc, _ := rarityOf(rar)
		sprite := sprites[slot%len(sprites)]
		nouns := spriteNouns[sprite]
		noun := nouns[(slot/len(sprites))%len(nouns)]
		// Перекос к нижней границе: дорогие экземпляры внутри редкости встречаются реже.
		base := rc.Lo + (rc.Hi-rc.Lo)*math.Pow(rng.Float64(), 1.6)
		if base >= 100 {
			base = math.Round(base)
		} else {
			base = round2(base)
		}
		c.Templates = append(c.Templates, genTemplate{
			ID:     "c" + itoa(k) + "-" + itoa(slot),
			Slot:   slot,
			Name:   words[slot%len(words)] + " " + noun,
			Rarity: rar,
			Art: map[string]any{
				"sprite": sprite, "theme": t.Key,
				"palette": rng.IntN(3), "bg": rng.IntN(3), "seed": rng.IntN(1000000),
			},
			Base:   base,
			Income: math.Round(base*rc.Yield/24*1e6) / 1e6,
			Supply: rc.Supply,
		})
		sums[rar] += base
		counts[rar]++
	}
	avg := func(r string) float64 {
		if counts[r] == 0 {
			return 0
		}
		return sums[r] / float64(counts[r])
	}
	for _, def := range packDefs {
		ev := 0.0
		if def.Kind == "mystery" {
			for _, rc := range Rarities {
				ev += avg(rc.Key) * float64(rc.Odds) / 1000
			}
		} else {
			for _, s := range def.Slots {
				ev += avg(s.Rarity) * float64(s.Count)
			}
		}
		// Запас 5% покрывает премию за ранние номера, сверху — наценка пака.
		c.Packs = append(c.Packs, genPack{
			ID: "c" + itoa(k) + "-" + def.Code, Code: def.Code, Name: def.Name, Kind: def.Kind,
			Caption: def.Caption, Slots: def.Slots, Stock: def.Stock,
			Price: nicePrice(ev * def.Markup * 1.05),
		})
	}
	return c
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

var (
	ensureMu    sync.Mutex
	ensuredDrop = -1
	ensuredDay  = ""
)

// Ensure лениво создаёт активные дропы и витрину xwallet на текущие сутки.
func Ensure(ctx context.Context, pool *pgxpool.Pool) error {
	now := time.Now().UTC()
	k := dropIndex(now)
	day := now.Format("2006-01-02")
	ensureMu.Lock()
	defer ensureMu.Unlock()
	if ensuredDrop == k && ensuredDay == day {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1735390078)`); err != nil {
		return err
	}
	for j := max(0, k-2); j <= k; j++ {
		if err = insertCollection(ctx, tx, generate(j)); err != nil {
			return err
		}
	}
	if err = insertGameListings(ctx, tx, now); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	ensuredDrop, ensuredDay = k, day
	return nil
}

func insertCollection(ctx context.Context, tx pgx.Tx, c genCollection) error {
	tag, err := tx.Exec(ctx, `
INSERT INTO nft_collections(id, name, theme, tagline, starts_at, ends_at)
VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
		c.ID, c.Name, c.Theme, c.Tagline, c.StartsAt, c.EndsAt)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	for _, t := range c.Templates {
		art, _ := json.Marshal(t.Art)
		if _, err = tx.Exec(ctx, `
INSERT INTO nft_templates(id, collection_id, slot, name, rarity, art, base_price, income_hour, supply)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO NOTHING`,
			t.ID, c.ID, t.Slot, t.Name, t.Rarity, art, t.Base, t.Income, t.Supply); err != nil {
			return err
		}
	}
	for _, p := range c.Packs {
		slots, _ := json.Marshal(p.Slots)
		if _, err = tx.Exec(ctx, `
INSERT INTO nft_packs(id, collection_id, code, name, kind, caption, slots, price, stock)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO NOTHING`,
			p.ID, c.ID, p.Code, p.Name, p.Kind, p.Caption, slots, p.Price, p.Stock); err != nil {
			return err
		}
	}
	return nil
}

// Витрина xwallet: не все шаблоны и не весь тираж, дорогие редкости появляются редко.
func insertGameListings(ctx context.Context, tx pgx.Tx, now time.Time) error {
	rows, err := tx.Query(ctx, `
SELECT t.id, t.rarity FROM nft_templates t
JOIN nft_collections c ON c.id = t.collection_id
WHERE c.starts_at <= $1 AND c.ends_at > $1 AND t.minted < t.supply`, now)
	if err != nil {
		return err
	}
	type pick struct {
		id, rarity string
	}
	var picks []pick
	for rows.Next() {
		var p pick
		if err = rows.Scan(&p.id, &p.rarity); err != nil {
			rows.Close()
			return err
		}
		picks = append(picks, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	dayNum := uint64(now.Sub(Epoch).Hours() / 24)
	day := now.Format("2006-01-02")
	for _, p := range picks {
		h := fnv.New64a()
		h.Write([]byte(p.id))
		rng := rand.New(rand.NewPCG(dayNum+7, h.Sum64()))
		rc, _ := rarityOf(p.rarity)
		if rng.Float64() >= rc.GameChance {
			continue
		}
		stock := rc.StockLo + rng.IntN(rc.StockHi-rc.StockLo+1)
		mult := math.Round((1.05+rng.Float64()*0.55)*1000) / 1000
		if _, err = tx.Exec(ctx, `
INSERT INTO nft_game_listings(day, template_id, mult, stock) VALUES ($1, $2, $3, $4)
ON CONFLICT (day, template_id) DO NOTHING`, day, p.id, mult, stock); err != nil {
			return err
		}
	}
	return nil
}

type UpcomingDrop struct {
	Name     string    `json:"name"`
	Theme    string    `json:"theme"`
	StartsAt time.Time `json:"startsAt"`
}

func NextDrop(now time.Time) UpcomingDrop {
	k := dropIndex(now) + 1
	return UpcomingDrop{Name: dropName(k), Theme: dropTheme(k).Key, StartsAt: dropStart(k)}
}
