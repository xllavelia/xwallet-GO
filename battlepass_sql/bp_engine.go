package battlepass_sql

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"math/big"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xwallet-server/nft_sql"
	"xwallet-server/profile_sql"
	"xwallet-server/referral_sql"
	"xwallet-server/user_vouchers_sql"
)

var (
	BPErrPassRequired     = errors.New("battle pass required")
	BPErrPassOwned        = errors.New("battle pass already owned")
	BPErrLevelLocked      = errors.New("level is locked")
	BPErrAlreadyClaimed   = errors.New("reward already claimed")
	BPErrInsufficientLavx = errors.New("not enough LAVX")
	BPErrVoucherSlotsFull = errors.New("voucher slots are full")
	BPErrQuestNotFound    = errors.New("quest not found")
	BPErrQuestNotComplete = errors.New("quest is not complete")
	BPErrQuestClaimed     = errors.New("quest already claimed")
	BPErrQuestExpired     = errors.New("quest expired")
	BPErrCaseNotFound     = errors.New("case not found")
	BPErrWalletMissing    = errors.New("wallet not found")
	BPErrReferralMissing  = errors.New("referral profile not found")
)

type BPSeasonState struct {
	ID       int64     `json:"id"`
	Label    string    `json:"label"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

type BPRewardCell struct {
	Rewards       []BPReward `json:"rewards"`
	Claimed       bool       `json:"claimed"`
	PendingCaseID int64      `json:"pendingCaseId,omitempty"`
}

type BPLevelState struct {
	Level    int          `json:"level"`
	Unlocked bool         `json:"unlocked"`
	Free     BPRewardCell `json:"free"`
	Pass     BPRewardCell `json:"pass"`
}

type BPPendingCase struct {
	ID     int64  `json:"id"`
	Rarity string `json:"rarity"`
	Title  string `json:"title"`
	Source string `json:"source"`
	Level  int    `json:"level"`
	Track  string `json:"track"`
}

type BPState struct {
	Season       BPSeasonState   `json:"season"`
	PassPrice    float64         `json:"passPrice"`
	LavxBalance  float64         `json:"lavxBalance"`
	PassOwned    bool            `json:"passOwned"`
	XP           int64           `json:"xp"`
	XPPerLevel   int             `json:"xpPerLevel"`
	MaxLevel     int             `json:"maxLevel"`
	Level        int             `json:"level"`
	LevelXP      int64           `json:"levelXp"`
	Levels       []BPLevelState  `json:"levels"`
	PendingCases []BPPendingCase `json:"pendingCases"`
	Statuses     []string        `json:"statuses"`
	CaseCatalog  BPCaseCatalog   `json:"caseCatalog"`
}

type BPPurchaseResult struct {
	PassOwned   bool    `json:"passOwned"`
	LavxBalance float64 `json:"lavxBalance"`
	PassPrice   float64 `json:"passPrice"`
}

type BPClaimResult struct {
	Level       int                `json:"level"`
	Track       string             `json:"track"`
	Rewards     []BPReward         `json:"rewards"`
	Notes       []string           `json:"notes,omitempty"`
	CaseTokenID int64              `json:"caseTokenId,omitempty"`
	CaseRarity  string             `json:"caseRarity,omitempty"`
	Nfts        []nft_sql.ItemView `json:"nfts,omitempty"`
	XP          int64              `json:"xp"`
	LevelAfter  int                `json:"levelAfter"`
	LevelXP     int64              `json:"levelXp"`
	LeveledUp   bool               `json:"leveledUp"`
}

type BPQuest struct {
	ID        int64   `json:"id"`
	Category  string  `json:"category"`
	Paid      bool    `json:"paid"`
	Title     string  `json:"title"`
	Progress  float64 `json:"progress"`
	Target    float64 `json:"target"`
	Unit      string  `json:"unit"`
	XP        int     `json:"xp"`
	Completed bool    `json:"completed"`
	Claimed   bool    `json:"claimed"`
}

type BPQuestTab struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	ResetsAt *time.Time `json:"resetsAt,omitempty"`
	Quests   []BPQuest  `json:"quests"`
}

type BPQuestsState struct {
	Season      BPSeasonState `json:"season"`
	PassOwned   bool          `json:"passOwned"`
	PassPrice   float64       `json:"passPrice"`
	LavxBalance float64       `json:"lavxBalance"`
	XP          int64         `json:"xp"`
	XPPerLevel  int           `json:"xpPerLevel"`
	Level       int           `json:"level"`
	LevelXP     int64         `json:"levelXp"`
	Tabs        []BPQuestTab  `json:"tabs"`
}

type BPQuestClaimResult struct {
	QuestID   int64 `json:"questId"`
	XPGained  int   `json:"xpGained"`
	XP        int64 `json:"xp"`
	Level     int   `json:"level"`
	LevelXP   int64 `json:"levelXp"`
	LeveledUp bool  `json:"leveledUp"`
}

type BPCaseRoll struct {
	Entry    BPCaseEntry `json:"entry"`
	Index    int         `json:"index"`
	Cosmetic string      `json:"cosmetic,omitempty"`
	Note     string      `json:"note,omitempty"`
	Granted  string      `json:"granted"`
}

type BPCaseOpenResult struct {
	Roll          BPCaseRoll `json:"roll"`
	AlreadyOpened bool       `json:"alreadyOpened"`
}

func BPLevelForXP(xp int64) (int, int64) {
	if xp < 0 {
		xp = 0
	}
	level := int(xp/int64(BPXpPerLevel)) + 1
	if level > BPMaxLevel {
		level = BPMaxLevel
	}
	if level == BPMaxLevel {
		return level, int64(BPXpPerLevel)
	}
	return level, xp - int64((level-1)*BPXpPerLevel)
}

func BPEnsureSeason(ctx context.Context, pool *pgxpool.Pool) (BPSeasonState, error) {
	var s BPSeasonState
	err := pool.QueryRow(ctx, `
SELECT id, label, starts_at, ends_at
FROM bp_seasons WHERE is_active ORDER BY id DESC LIMIT 1;
`).Scan(&s.ID, &s.Label, &s.StartsAt, &s.EndsAt)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return s, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(918273645);`); err != nil {
		return s, err
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(918273645);`) }()

	err = conn.QueryRow(ctx, `
SELECT id, label, starts_at, ends_at
FROM bp_seasons WHERE is_active ORDER BY id DESC LIMIT 1;
`).Scan(&s.ID, &s.Label, &s.StartsAt, &s.EndsAt)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}

	now := time.Now().UTC()
	err = conn.QueryRow(ctx, `
INSERT INTO bp_seasons(label, starts_at, ends_at, is_active)
VALUES ($1, $2, $3, true)
RETURNING id, label, starts_at, ends_at;
`, BPSeasonLabel, now, now.Add(BPSeasonDuration)).Scan(&s.ID, &s.Label, &s.StartsAt, &s.EndsAt)
	return s, err
}

func bpEnsureProfileTx(ctx context.Context, tx pgx.Tx, userID int, seasonID int64) (int64, error) {
	var xp int64
	err := tx.QueryRow(ctx, `
INSERT INTO bp_profiles(user_id, season_id, xp, updated_at)
VALUES ($1, $2, 0, now())
ON CONFLICT (user_id) DO UPDATE
SET season_id = EXCLUDED.season_id, xp = 0, updated_at = now()
WHERE bp_profiles.season_id <> EXCLUDED.season_id
RETURNING xp;
`, userID, seasonID).Scan(&xp)
	if err == nil {
		return xp, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRow(ctx, `SELECT xp FROM bp_profiles WHERE user_id = $1;`, userID).Scan(&xp)
	return xp, err
}

func bpAddXPTx(ctx context.Context, tx pgx.Tx, userID int, amount int) (int64, error) {
	var xp int64
	err := tx.QueryRow(ctx, `
UPDATE bp_profiles SET xp = xp + $1, updated_at = now()
WHERE user_id = $2 RETURNING xp;
`, amount, userID).Scan(&xp)
	return xp, err
}

func BPLoadState(ctx context.Context, pool *pgxpool.Pool, userID int) (*BPState, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}
	level, levelXP := BPLevelForXP(xp)

	var passOwned bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2);
`, userID, season.ID).Scan(&passOwned); err != nil {
		return nil, err
	}

	var lavx float64
	err = tx.QueryRow(ctx, `SELECT COALESCE(lavx_balance, 0) FROM wallets WHERE user_id = $1;`, userID).Scan(&lavx)
	if errors.Is(err, pgx.ErrNoRows) {
		lavx = 0
	} else if err != nil {
		return nil, err
	}

	claimed := map[string]bool{}
	rows, err := tx.Query(ctx, `
SELECT level, track FROM bp_claims WHERE user_id = $1 AND season_id = $2;
`, userID, season.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var lvl int
		var track string
		if err := rows.Scan(&lvl, &track); err != nil {
			rows.Close()
			return nil, err
		}
		claimed[fmt.Sprintf("%d:%s", lvl, track)] = true
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	pending := []BPPendingCase{}
	pendingCell := map[string]int64{}
	rows, err = tx.Query(ctx, `
SELECT id, rarity, source_level, source_track
FROM bp_case_tokens
WHERE user_id = $1 AND season_id = $2 AND opened_at IS NULL
ORDER BY id;
`, userID, season.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p BPPendingCase
		if err := rows.Scan(&p.ID, &p.Rarity, &p.Level, &p.Track); err != nil {
			rows.Close()
			return nil, err
		}
		p.Source = "level"
		p.Title = bpCaseTitle(p.Rarity) + " case"
		pending = append(pending, p)
		if p.Level > 0 {
			pendingCell[fmt.Sprintf("%d:%s", p.Level, p.Track)] = p.ID
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	statuses := []string{}
	rows, err = tx.Query(ctx, `SELECT status FROM user_statuses WHERE user_id = $1 ORDER BY granted_at;`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		statuses = append(statuses, s)
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	levels := make([]BPLevelState, 0, BPMaxLevel)
	for i := 1; i <= BPMaxLevel; i++ {
		row := BPLevelRewardsFor(i)
		free := BPRewardCell{Rewards: row.Free, Claimed: claimed[fmt.Sprintf("%d:free", i)]}
		pass := BPRewardCell{Rewards: row.Pass, Claimed: claimed[fmt.Sprintf("%d:pass", i)]}
		if free.Rewards == nil {
			free.Rewards = []BPReward{}
		}
		if pass.Rewards == nil {
			pass.Rewards = []BPReward{}
		}
		if id := pendingCell[fmt.Sprintf("%d:free", i)]; id > 0 {
			free.PendingCaseID = id
		}
		if id := pendingCell[fmt.Sprintf("%d:pass", i)]; id > 0 {
			pass.PendingCaseID = id
		}
		levels = append(levels, BPLevelState{
			Level:    i,
			Unlocked: i <= level,
			Free:     free,
			Pass:     pass,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BPState{
		Season:       season,
		PassPrice:    BPPassPriceLavx,
		LavxBalance:  lavx,
		PassOwned:    passOwned,
		XP:           xp,
		XPPerLevel:   BPXpPerLevel,
		MaxLevel:     BPMaxLevel,
		Level:        level,
		LevelXP:      levelXP,
		Levels:       levels,
		PendingCases: pending,
		Statuses:     statuses,
		CaseCatalog:  BPCaseCatalog{Order: append([]string(nil), BPCaseOrder...), Cases: BPCases},
	}, nil
}

func BPRecordEvent(ctx context.Context, pool *pgxpool.Pool, userID int, eventKey, metric string, amount float64) error {
	if userID <= 0 || eventKey == "" || metric == "" {
		return nil
	}
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
INSERT INTO bp_events(user_id, season_id, event_key, metric, amount, occurred_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (user_id, season_id, event_key) DO NOTHING;
`, userID, season.ID, eventKey, metric, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	keys := BPCurrentPeriodKeys(time.Now().UTC())
	_, err = tx.Exec(ctx, `
WITH n AS (
	SELECT id, CASE mode
		WHEN 'count' THEN progress + $5
		WHEN 'sum' THEN GREATEST(0, progress + $5)
		ELSE GREATEST(progress, $5)
	END AS value
	FROM bp_quests
	WHERE user_id = $1 AND season_id = $2 AND metric = $3
		AND claimed_at IS NULL AND period_key = ANY($4)
)
UPDATE bp_quests q
SET progress = n.value,
	completed_at = CASE
		WHEN q.completed_at IS NULL AND n.value >= q.target THEN now()
		ELSE q.completed_at
	END
FROM n WHERE q.id = n.id;
`, userID, season.ID, metric, keys, amount)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func BPTrackEvent(ctx context.Context, pool *pgxpool.Pool, userID int, eventKey, metric string, amount float64) {
	if err := BPRecordEvent(ctx, pool, userID, eventKey, metric, amount); err != nil {
		log.Printf("battlepass event %s/%s user=%d: %v", eventKey, metric, userID, err)
	}
}

func bpSeed(parts ...interface{}) int64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%v|", p)
	}
	return int64(h.Sum64())
}

func bpShuffled(rng *rand.Rand, pool []BPQuestTemplate) []BPQuestTemplate {
	out := append([]BPQuestTemplate(nil), pool...)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func bpPickTarget(rng *rand.Rand, t BPQuestTemplate) float64 {
	if t.Step <= 0 || t.Max <= t.Min {
		return t.Min
	}
	steps := int(math.Round((t.Max-t.Min)/t.Step)) + 1
	if steps < 1 {
		steps = 1
	}
	return t.Min + float64(rng.Intn(steps))*t.Step
}

func bpFormatTarget(t BPQuestTemplate, v float64) string {
	n := bpNum(v)
	switch t.Unit {
	case "usd":
		return "$" + n
	case "xp":
		return n + " XP"
	case "x":
		return n + "x"
	case "power":
		return n + " Power"
	}
	return n
}

func bpEnsureQuestsTx(ctx context.Context, tx pgx.Tx, userID int, seasonID int64, now time.Time) error {
	for _, category := range []string{"daily", "weekly", "monthly"} {
		period := BPPeriod(category, now)
		normal, paidCount := BPQuestCounts(category)
		total := normal + paidCount

		if _, err := tx.Exec(ctx, `
DELETE FROM bp_quests
WHERE user_id = $1 AND season_id = $2 AND category = $3 AND period_key = $4
	AND slot > $5 AND claimed_at IS NULL;
`, userID, seasonID, category, period.Key, total); err != nil {
			return err
		}

		existing := map[int]bool{}
		used := map[string]bool{}
		rows, err := tx.Query(ctx, `
SELECT slot, template_id FROM bp_quests
WHERE user_id = $1 AND season_id = $2 AND category = $3 AND period_key = $4;
`, userID, seasonID, category, period.Key)
		if err != nil {
			return err
		}
		for rows.Next() {
			var slot int
			var templateID string
			if err := rows.Scan(&slot, &templateID); err != nil {
				rows.Close()
				return err
			}
			existing[slot] = true
			used[templateID] = true
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}

		allPresent := true
		for slot := 1; slot <= total; slot++ {
			if !existing[slot] {
				allPresent = false
				break
			}
		}
		if allPresent {
			continue
		}

		rng := rand.New(rand.NewSource(bpSeed(userID, seasonID, category, period.Key)))
		normalOrder := bpShuffled(rng, BPQuestPool(category, false))
		paidOrder := bpShuffled(rng, BPQuestPool(category, true))
		normalIdx, paidIdx := 0, 0

		nextTemplate := func(order []BPQuestTemplate, idx *int) (BPQuestTemplate, bool) {
			for *idx < len(order) {
				t := order[*idx]
				*idx = *idx + 1
				if !used[t.ID] {
					return t, true
				}
			}
			return BPQuestTemplate{}, false
		}

		for slot := 1; slot <= total; slot++ {
			if existing[slot] {
				continue
			}
			isPaid := slot > normal
			order := normalOrder
			idx := &normalIdx
			if isPaid {
				order = paidOrder
				idx = &paidIdx
			}
			t, ok := nextTemplate(order, idx)
			if !ok {
				continue
			}
			used[t.ID] = true
			target := bpPickTarget(rng, t)
			title := fmt.Sprintf(t.Title, bpFormatTarget(t, target))
			xp := BPQuestXP(category, isPaid)

			var questID int64
			err := tx.QueryRow(ctx, `
INSERT INTO bp_quests(
	user_id, season_id, category, period_key, period_start, period_end,
	slot, template_id, title, metric, mode, target, progress, unit, xp, paid
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,0,$13,$14,$15)
ON CONFLICT (user_id, season_id, category, period_key, slot) DO NOTHING
RETURNING id;
`, userID, seasonID, category, period.Key, period.Start, period.End,
				slot, t.ID, title, t.Metric, t.Mode, target, t.Unit, xp, isPaid).Scan(&questID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if err := bpBackfillQuestTx(ctx, tx, questID, userID, seasonID, t.Metric, t.Mode, period.Start, period.End); err != nil {
				return err
			}
		}
	}
	return nil
}

func bpBackfillQuestTx(ctx context.Context, tx pgx.Tx, questID int64, userID int, seasonID int64, metric, mode string, start, end time.Time) error {
	var query string
	switch mode {
	case "max":
		query = `SELECT COALESCE(MAX(amount), 0)::float8 FROM bp_events WHERE user_id=$1 AND season_id=$2 AND metric=$3 AND occurred_at >= $4 AND occurred_at < $5;`
	default:
		// count и sum: amount хранит количество единиц за событие,
		// поэтому бэкфилл суммирует, как и живое обновление прогресса.
		query = `SELECT COALESCE(SUM(amount), 0)::float8 FROM bp_events WHERE user_id=$1 AND season_id=$2 AND metric=$3 AND occurred_at >= $4 AND occurred_at < $5;`
	}
	var value float64
	if err := tx.QueryRow(ctx, query, userID, seasonID, metric, start, end).Scan(&value); err != nil {
		return err
	}
	if value < 0 {
		value = 0
	}
	_, err := tx.Exec(ctx, `
UPDATE bp_quests
SET progress = $1,
	completed_at = CASE WHEN completed_at IS NULL AND $1 >= target THEN now() ELSE completed_at END
WHERE id = $2;
`, value, questID)
	return err
}

func BPLoadQuests(ctx context.Context, pool *pgxpool.Pool, userID int) (*BPQuestsState, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}
	level, levelXP := BPLevelForXP(xp)

	if err := bpEnsureQuestsTx(ctx, tx, userID, season.ID, time.Now().UTC()); err != nil {
		return nil, err
	}

	var passOwned bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2);
`, userID, season.ID).Scan(&passOwned); err != nil {
		return nil, err
	}
	var lavx float64
	err = tx.QueryRow(ctx, `SELECT COALESCE(lavx_balance, 0) FROM wallets WHERE user_id = $1;`, userID).Scan(&lavx)
	if errors.Is(err, pgx.ErrNoRows) {
		lavx = 0
	} else if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	tabs := make([]BPQuestTab, 0, 4)
	passQuests := []BPQuest{}

	for _, category := range []string{"daily", "weekly", "monthly"} {
		period := BPPeriod(category, now)
		rows, err := tx.Query(ctx, `
SELECT id, paid, title, progress, target, unit, xp, completed_at, claimed_at
FROM bp_quests
WHERE user_id = $1 AND season_id = $2 AND category = $3 AND period_key = $4
ORDER BY slot;
`, userID, season.ID, category, period.Key)
		if err != nil {
			return nil, err
		}
		quests := []BPQuest{}
		for rows.Next() {
			var q BPQuest
			var completedAt, claimedAt *time.Time
			q.Category = category
			if err := rows.Scan(&q.ID, &q.Paid, &q.Title, &q.Progress, &q.Target, &q.Unit, &q.XP, &completedAt, &claimedAt); err != nil {
				rows.Close()
				return nil, err
			}
			q.Completed = completedAt != nil
			q.Claimed = claimedAt != nil
			quests = append(quests, q)
			if q.Paid {
				passQuests = append(passQuests, q)
			}
		}
		rows.Close()
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		reset := period.End
		tabs = append(tabs, BPQuestTab{
			Key:      category,
			Label:    BPQuestCategoryLabel(category),
			ResetsAt: &reset,
			Quests:   quests,
		})
	}
	tabs = append(tabs, BPQuestTab{Key: "pass", Label: "Pass", Quests: passQuests})

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BPQuestsState{
		Season:      season,
		PassOwned:   passOwned,
		PassPrice:   BPPassPriceLavx,
		LavxBalance: lavx,
		XP:          xp,
		XPPerLevel:  BPXpPerLevel,
		Level:       level,
		LevelXP:     levelXP,
		Tabs:        tabs,
	}, nil
}

func BPPurchasePass(ctx context.Context, pool *pgxpool.Pool, userID int) (*BPPurchaseResult, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var owned bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2);
`, userID, season.ID).Scan(&owned); err != nil {
		return nil, err
	}
	if owned {
		return nil, BPErrPassOwned
	}

	var lavx float64
	err = tx.QueryRow(ctx, `SELECT lavx_balance FROM wallets WHERE user_id = $1 FOR UPDATE;`, userID).Scan(&lavx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, BPErrWalletMissing
	}
	if err != nil {
		return nil, err
	}
	if lavx < BPPassPriceLavx {
		return nil, BPErrInsufficientLavx
	}

	if _, err := tx.Exec(ctx, `
UPDATE wallets SET lavx_balance = lavx_balance - $1, updated_at = now()
WHERE user_id = $2;
`, BPPassPriceLavx, userID); err != nil {
		return nil, err
	}

	tag, err := tx.Exec(ctx, `
INSERT INTO bp_passes(user_id, season_id) VALUES ($1, $2)
ON CONFLICT (user_id, season_id) DO NOTHING;
`, userID, season.ID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, BPErrPassOwned
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &BPPurchaseResult{
		PassOwned:   true,
		LavxBalance: lavx - BPPassPriceLavx,
		PassPrice:   BPPassPriceLavx,
	}, nil
}

func BPClaimReward(ctx context.Context, pool *pgxpool.Pool, userID int, level int, track string) (*BPClaimResult, error) {
	track = strings.ToLower(strings.TrimSpace(track))
	if track != "free" && track != "pass" {
		return nil, fmt.Errorf("invalid track")
	}
	if level < 1 || level > BPMaxLevel {
		return nil, fmt.Errorf("invalid level")
	}
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	if _, err := referral_sql.GetOrCreateByUserID(ctx, pool, userID); err != nil {
		return nil, err
	}
	if err := nft_sql.Ensure(ctx, pool); err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}
	currentLevel, _ := BPLevelForXP(xp)
	if level > currentLevel {
		return nil, BPErrLevelLocked
	}

	if track == "pass" {
		var owned bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2);
`, userID, season.ID).Scan(&owned); err != nil {
			return nil, err
		}
		if !owned {
			return nil, BPErrPassRequired
		}
	}

	tag, err := tx.Exec(ctx, `
INSERT INTO bp_claims(user_id, season_id, level, track)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, season_id, level, track) DO NOTHING;
`, userID, season.ID, level, track)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, BPErrAlreadyClaimed
	}

	row := BPLevelRewardsFor(level)
	rewards := row.Free
	if track == "pass" {
		rewards = row.Pass
	}
	notes := []string{}
	var caseTokenID int64
	var caseRarity string
	nfts := []nft_sql.ItemView{}
	for _, r := range rewards {
		if r.Kind == "nft" {
			items, note, err := nft_sql.GrantTx(ctx, tx, userID, nft_sql.GrantSpec{Rarity: r.Rarity, Count: r.Count, Pack: r.Pack}, "Battle Pass")
			if err != nil {
				return nil, err
			}
			nfts = append(nfts, items...)
			if note != "" {
				notes = append(notes, note)
			}
			continue
		}
		tokenID, note, err := bpGrantRewardTx(ctx, tx, userID, season.ID, r, level, track, false)
		if err != nil {
			return nil, err
		}
		if tokenID > 0 {
			caseTokenID = tokenID
		}
		if r.Kind == "case" {
			caseRarity = r.Case
		}
		if note != "" {
			notes = append(notes, note)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BPClaimResult{
		Level:       level,
		Track:       track,
		Rewards:     rewards,
		Notes:       notes,
		CaseTokenID: caseTokenID,
		CaseRarity:  caseRarity,
		Nfts:        nfts,
		XP:          xp,
		LevelAfter:  currentLevel,
		LevelXP:     func() int64 { _, lx := BPLevelForXP(xp); return lx }(),
		LeveledUp:   false,
	}, nil
}

func bpGrantRewardTx(ctx context.Context, tx pgx.Tx, userID int, seasonID int64, r BPReward, level int, track string, convertFull bool) (int64, string, error) {
	cosmeticSource := "battlepass"
	if convertFull {
		cosmeticSource = "case"
	}
	switch r.Kind {
	case "cash":
		tag, err := tx.Exec(ctx, `
UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;
`, r.Value, userID)
		if err != nil {
			return 0, "", err
		}
		if tag.RowsAffected() == 0 {
			return 0, "", BPErrWalletMissing
		}
	case "lavx":
		tag, err := tx.Exec(ctx, `
UPDATE wallets SET lavx_balance = lavx_balance + $1, updated_at = now() WHERE user_id = $2;
`, r.Value, userID)
		if err != nil {
			return 0, "", err
		}
		if tag.RowsAffected() == 0 {
			return 0, "", BPErrWalletMissing
		}
	case "ref":
		tag, err := tx.Exec(ctx, `UPDATE referrals SET ref_xp = ref_xp + $1 WHERE user_id = $2;`, int(r.Value), userID)
		if err != nil {
			return 0, "", err
		}
		if tag.RowsAffected() == 0 {
			return 0, "", BPErrReferralMissing
		}
	case "fee":
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count); err != nil {
			return 0, "", err
		}
		if count >= BPVoucherSlots {
			if !convertFull {
				return 0, "", BPErrVoucherSlotsFull
			}
			if _, err := tx.Exec(ctx, `
UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;
`, r.Value, userID); err != nil {
				return 0, "", err
			}
			return 0, BPFormatReward(r) + " added as cash (voucher slots full)", nil
		}
		if err := user_vouchers_sql.GrantFeeDiscountVoucherTx(ctx, tx, userID, r.Value, r.Days*86400, "battlepass", BPVoucherSlots); err != nil {
			if errors.Is(err, user_vouchers_sql.ErrSlotsFull) {
				return 0, "", BPErrVoucherSlotsFull
			}
			return 0, "", err
		}
	case "case":
		payload, _ := json.Marshal(r)
		var id int64
		err := tx.QueryRow(ctx, `
INSERT INTO bp_case_tokens(user_id, season_id, rarity, source, source_level, source_track, reward)
VALUES ($1, $2, $3, 'level', $4, $5, $6)
RETURNING id;
`, userID, seasonID, r.Case, level, track, payload).Scan(&id)
		if err != nil {
			return 0, "", err
		}
		return id, "", nil
	case "cosmetic":
		cosmetic, ok := profile_sql.GetCosmetic(r.Cosmetic)
		if !ok || cosmetic.AdminOnly || r.Value != 0 {
			return 0, "", fmt.Errorf("invalid cosmetic reward %q", r.Cosmetic)
		}
		if _, err := profile_sql.GrantCosmeticTx(ctx, tx, userID, cosmetic.ID, cosmeticSource); err != nil {
			return 0, "", err
		}
	case "custom":
		if r.Voucher == "" {
			return 0, "", fmt.Errorf("invalid custom voucher reward")
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_vouchers WHERE user_id = $1;`, userID).Scan(&count); err != nil {
			return 0, "", err
		}
		if count >= BPVoucherSlots {
			if !convertFull {
				return 0, "", BPErrVoucherSlotsFull
			}
			if _, err := tx.Exec(ctx, `
UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2;
`, BPDuplicateStatusCash, userID); err != nil {
				return 0, "", err
			}
			return 0, BPFormatReward(r) + " added as cash (voucher slots full)", nil
		}
		if err := user_vouchers_sql.GrantCustomVoucherTx(ctx, tx, userID, r.Voucher, cosmeticSource, BPVoucherSlots); err != nil {
			if errors.Is(err, user_vouchers_sql.ErrSlotsFull) {
				return 0, "", BPErrVoucherSlotsFull
			}
			return 0, "", err
		}
	default:
		return 0, "", fmt.Errorf("unknown reward kind %q", r.Kind)
	}
	return 0, "", nil
}

func BPClaimQuest(ctx context.Context, pool *pgxpool.Pool, userID int, questID int64) (*BPQuestClaimResult, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	xp, err := bpEnsureProfileTx(ctx, tx, userID, season.ID)
	if err != nil {
		return nil, err
	}

	var category, periodKey string
	var paid bool
	var xpReward int
	var completedAt, claimedAt *time.Time
	err = tx.QueryRow(ctx, `
SELECT category, period_key, paid, xp, completed_at, claimed_at
FROM bp_quests
WHERE id = $1 AND user_id = $2 AND season_id = $3
FOR UPDATE;
`, questID, userID, season.ID).Scan(&category, &periodKey, &paid, &xpReward, &completedAt, &claimedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, BPErrQuestNotFound
	}
	if err != nil {
		return nil, err
	}
	if claimedAt != nil {
		return nil, BPErrQuestClaimed
	}
	if completedAt == nil {
		return nil, BPErrQuestNotComplete
	}
	if BPPeriodKey(category, time.Now().UTC()) != periodKey {
		return nil, BPErrQuestExpired
	}
	if paid {
		var owned bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM bp_passes WHERE user_id = $1 AND season_id = $2);
`, userID, season.ID).Scan(&owned); err != nil {
			return nil, err
		}
		if !owned {
			return nil, BPErrPassRequired
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE bp_quests SET claimed_at = now() WHERE id = $1;`, questID); err != nil {
		return nil, err
	}
	newXP, err := bpAddXPTx(ctx, tx, userID, xpReward)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	oldLevel, _ := BPLevelForXP(xp)
	newLevel, levelXP := BPLevelForXP(newXP)
	return &BPQuestClaimResult{
		QuestID:   questID,
		XPGained:  xpReward,
		XP:        newXP,
		Level:     newLevel,
		LevelXP:   levelXP,
		LeveledUp: newLevel > oldLevel,
	}, nil
}

func bpRollCase(def BPCaseDef) (BPCaseEntry, int, error) {
	roll, err := cryptorand.Int(cryptorand.Reader, big.NewInt(100))
	if err != nil {
		return BPCaseEntry{}, 0, err
	}
	n := int(roll.Int64())
	cum := 0
	for i, e := range def.Entries {
		cum += e.Chance
		if n < cum {
			return e, i, nil
		}
	}
	return BPCaseEntry{}, 0, fmt.Errorf("invalid case chances for %q", def.Rarity)
}

func BPOpenCase(ctx context.Context, pool *pgxpool.Pool, userID int, tokenID int64) (*BPCaseOpenResult, error) {
	season, err := BPEnsureSeason(ctx, pool)
	if err != nil {
		return nil, err
	}
	if _, err := referral_sql.GetOrCreateByUserID(ctx, pool, userID); err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var rarity string
	var level int
	var track string
	var openedAt *time.Time
	var rollRaw []byte
	err = tx.QueryRow(ctx, `
SELECT rarity, source_level, source_track, opened_at, COALESCE(roll, 'null'::jsonb)
FROM bp_case_tokens
WHERE id = $1 AND user_id = $2 AND season_id = $3
FOR UPDATE;
`, tokenID, userID, season.ID).Scan(&rarity, &level, &track, &openedAt, &rollRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, BPErrCaseNotFound
	}
	if err != nil {
		return nil, err
	}
	if openedAt != nil {
		var roll BPCaseRoll
		if len(rollRaw) > 0 {
			_ = json.Unmarshal(rollRaw, &roll)
		}
		return &BPCaseOpenResult{Roll: roll, AlreadyOpened: true}, nil
	}

	def, ok := BPCases[rarity]
	if !ok {
		return nil, BPErrCaseNotFound
	}
	entry, idx, err := bpRollCase(def)
	if err != nil {
		return nil, err
	}

	reward := BPReward{
		Kind: entry.Kind, Value: entry.Value, Days: entry.Days, Voucher: entry.Voucher,
		Cosmetic: entry.Cosmetic, CosmeticKind: entry.CosmeticKind,
		Label: entry.Label, Display: entry.Display,
	}
	_, note, err := bpGrantRewardTx(ctx, tx, userID, season.ID, reward, level, track, true)
	if err != nil {
		return nil, err
	}

	granted := entry.Display
	if note != "" {
		granted = note
	}
	roll := BPCaseRoll{Entry: entry, Index: idx, Cosmetic: entry.Cosmetic, Note: note, Granted: granted}
	raw, _ := json.Marshal(roll)
	if _, err := tx.Exec(ctx, `
UPDATE bp_case_tokens SET opened_at = now(), roll = $1 WHERE id = $2;
`, raw, tokenID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &BPCaseOpenResult{Roll: roll, AlreadyOpened: false}, nil
}
