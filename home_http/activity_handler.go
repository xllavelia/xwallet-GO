package home_http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"xwallet-server/auth_http"
	"xwallet-server/battlepass_sql"
	"xwallet-server/nft_sql"
	"xwallet-server/rewards_sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type activityFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

type activityEvent struct {
	ID     string         `json:"id"`
	Source string         `json:"source"`
	Kind   string         `json:"kind"`
	Title  string         `json:"title"`
	Amount float64        `json:"amount"`
	Asset  string         `json:"asset"`
	At     time.Time      `json:"at"`
	Facts  []activityFact `json:"facts"`
}

type activityTicketPack struct {
	ID           string     `json:"id"`
	Rarity       string     `json:"rarity"`
	Cost         float64    `json:"cost"`
	Won          float64    `json:"won"`
	Opened       int        `json:"opened"`
	Total        int        `json:"total"`
	PurchasedAt  time.Time  `json:"purchasedAt"`
	LastOpenedAt *time.Time `json:"lastOpenedAt,omitempty"`
}

type activityResponse struct {
	Events      []activityEvent      `json:"events"`
	TicketPacks []activityTicketPack `json:"ticketPacks"`
	Coverage    []string             `json:"coverage"`
}

func newActivityResponse() activityResponse {
	return activityResponse{
		Events:      []activityEvent{},
		TicketPacks: []activityTicketPack{},
		Coverage: []string{
			"Slots purchase dates are not recorded; dated totals include payouts only.",
			"Slots payouts are calendar-day totals; midnight UTC represents the recorded date, not an individual payout time.",
			"Daily and battle pass level rewards and pass prices use current catalogs; historical catalog versions are not recorded.",
			"Daily voucher issuance can be skipped when slots are full; claims do not record that outcome. Battle pass level status grants or duplicate cash conversions are not recorded; status values are not counted as cash. New cosmetic rewards are not inferred from historical level claims.",
			"Legacy TIMESTAMP fields have no stored timezone and are exposed as UTC. Rocket and Flip payouts use round ended_at; individual payout timestamps are not recorded.",
			"NFT history follows mint, sale and upgrade records. Hourly NFT income is not listed as separate events, and NFTs you no longer own appear only as sales or upgrade attempts.",
		},
	}
}

func GetActivityHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user, ok := auth_http.UserFromContext(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			http.Error(w, "could not load activity", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(ctx)

		var userID int
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE player_id = $1;`, user.PlayerID).Scan(&userID); err != nil {
			http.Error(w, "could not load activity", http.StatusInternalServerError)
			return
		}
		response, err := loadActivity(ctx, tx, userID)
		if err != nil {
			http.Error(w, "could not load activity", http.StatusInternalServerError)
			return
		}
		body, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "could not encode activity", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			http.Error(w, "could not load activity", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	}
}

func loadActivity(ctx context.Context, tx pgx.Tx, userID int) (activityResponse, error) {
	response := newActivityResponse()
	for _, load := range []func(context.Context, pgx.Tx, int, *activityResponse) error{
		loadDailyActivity, loadBattlepassActivity, loadCaseActivity, loadQuestActivity,
		loadTicketActivity, loadBetActivity, loadSlotsActivity, loadMarketActivity, loadVoucherActivity,
		loadNftActivity,
	} {
		if err := load(ctx, tx, userID, &response); err != nil {
			return activityResponse{}, err
		}
	}
	sort.Slice(response.Events, func(i, j int) bool {
		a, b := response.Events[i], response.Events[j]
		if a.At.Equal(b.At) {
			return a.ID < b.ID
		}
		return a.At.After(b.At)
	})
	return response, nil
}

func activityRows(ctx context.Context, tx pgx.Tx, userID int, query string, scan func(pgx.Rows) error) error {
	rows, err := tx.Query(ctx, query, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func activityNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func activityFactFor(label, value string) activityFact {
	return activityFact{Label: label, Value: value}
}

func makeActivityEvent(id, source, kind, title, asset string, amount float64, at time.Time, facts ...activityFact) activityEvent {
	if facts == nil {
		facts = []activityFact{}
	}
	return activityEvent{ID: id, Source: source, Kind: kind, Title: title, Asset: asset, Amount: amount, At: at.UTC(), Facts: facts}
}

func dailyActivityEvents(id int64, track string, day int, at time.Time) ([]activityEvent, error) {
	catalog := rewards_sql.CatalogFor(track)
	var reward *rewards_sql.DayReward
	for i := range catalog {
		if catalog[i].Day == day {
			reward = &catalog[i]
			break
		}
	}
	if reward == nil {
		return nil, fmt.Errorf("unknown daily reward %s/%d", track, day)
	}
	events := []activityEvent{}
	for i, comp := range reward.Components {
		asset, kind := "", "reward"
		switch comp.Kind {
		case "usdt":
			asset = "USD"
		case "lavx":
			asset = "LAVX"
		case "ref_xp":
			asset = "REF XP"
		case "pass_xp":
			asset = "XP"
		case "voucher":
			asset, kind = "voucher", "voucher_issued"
		case "nft":
			// NFT добавлены позже — старые клеймы их не получали, видны в инвентаре NFT
			continue
		default:
			return nil, fmt.Errorf("unknown daily component %q", comp.Kind)
		}
		events = append(events, makeActivityEvent(fmt.Sprintf("daily:%d:%d", id, i), "daily", kind,
			fmt.Sprintf("Day %d reward", day), asset, math.Abs(comp.Value), at,
			activityFactFor("Track", track), activityFactFor("Day", strconv.Itoa(day)), activityFactFor("Reward", comp.Kind)))
	}
	return events, nil
}

func loadDailyActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT id, track, day, claimed_at FROM daily_reward_claims WHERE user_id = $1;
	`, func(rows pgx.Rows) error {
		var id int64
		var track string
		var day int
		var at time.Time
		if err := rows.Scan(&id, &track, &day, &at); err != nil {
			return err
		}
		events, err := dailyActivityEvents(id, track, day, at)
		if err != nil {
			return err
		}
		response.Events = append(response.Events, events...)
		return nil
	})
}

func battlepassRewardAmount(reward battlepass_sql.BPReward) (string, string, float64, error) {
	switch reward.Kind {
	case "cash":
		return "USD", "reward", math.Abs(reward.Value), nil
	case "lavx":
		return "LAVX", "reward", math.Abs(reward.Value), nil
	case "ref":
		return "REF XP", "reward", math.Abs(reward.Value), nil
	case "fee":
		return "voucher", "voucher_issued", math.Abs(reward.Value), nil
	case "case":
		return "case", "case_issued", 1, nil
	case "cosmetic":
		return "cosmetic", "cosmetic", 1, nil
	case "custom":
		return "voucher", "voucher_issued", 0, nil
	case "status":
		// Легаси-записи старых кейсов: статусы больше не выдаются, но история отображается.
		return "cosmetic", "cosmetic", 1, nil
	default:
		return "", "", 0, fmt.Errorf("unknown battle pass reward %q", reward.Kind)
	}
}

func battlepassActivityEvents(userID int, seasonID int64, season string, level int, track string, at time.Time) ([]activityEvent, error) {
	catalog := battlepass_sql.BPLevelRewardsFor(level)
	var rewards []battlepass_sql.BPReward
	switch track {
	case "free":
		rewards = catalog.Free
	case "pass":
		rewards = catalog.Pass
	default:
		return nil, fmt.Errorf("unknown battle pass track %q", track)
	}
	if len(rewards) == 0 {
		return nil, fmt.Errorf("unknown battle pass level %d", level)
	}
	events := []activityEvent{}
	for i, reward := range rewards {
		// Старые клеймы не содержат снимок наград и не доказывают новое открытие.
		if reward.Kind == "cosmetic" || reward.Kind == "nft" {
			continue
		}
		asset, kind, amount, err := battlepassRewardAmount(reward)
		if err != nil {
			return nil, err
		}
		facts := []activityFact{activityFactFor("Season", season), activityFactFor("Level", strconv.Itoa(level)),
			activityFactFor("Track", track), activityFactFor("Reward", battlepass_sql.BPFormatReward(reward))}
		events = append(events, makeActivityEvent(fmt.Sprintf("bp:claim:%d:%d:%d:%s:%d", userID, seasonID, level, track, i),
			"battlepass", kind, fmt.Sprintf("Level %d reward", level),
			asset, amount, at, facts...))
	}
	return events, nil
}

func loadBattlepassActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	if err := activityRows(ctx, tx, userID, `
		SELECT c.season_id, s.label, c.level, c.track, c.claimed_at
		FROM bp_claims c JOIN bp_seasons s ON s.id = c.season_id WHERE c.user_id = $1;
	`, func(rows pgx.Rows) error {
		var seasonID int64
		var season, track string
		var level int
		var at time.Time
		if err := rows.Scan(&seasonID, &season, &level, &track, &at); err != nil {
			return err
		}
		events, err := battlepassActivityEvents(userID, seasonID, season, level, track, at)
		if err != nil {
			return err
		}
		response.Events = append(response.Events, events...)
		return nil
	}); err != nil {
		return err
	}
	return activityRows(ctx, tx, userID, `
		SELECT p.season_id, s.label, p.purchased_at
		FROM bp_passes p JOIN bp_seasons s ON s.id = p.season_id WHERE p.user_id = $1;
	`, func(rows pgx.Rows) error {
		var seasonID int64
		var season string
		var at time.Time
		if err := rows.Scan(&seasonID, &season, &at); err != nil {
			return err
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("bp:pass:%d:%d", userID, seasonID),
			"battlepass", "purchase", season+" · pass purchase", "LAVX", -battlepass_sql.BPPassPriceLavx, at,
			activityFactFor("Season", season)))
		return nil
	})
}

func storedCaseCash(text string) (float64, error) {
	amount, err := strconv.ParseFloat(text, 64)
	if err != nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("invalid stored case cash %q", text)
	}
	return amount, nil
}

func caseActivityAmount(roll battlepass_sql.BPCaseRoll) (string, string, float64, error) {
	texts := []string{roll.Note, roll.Granted}
	for _, text := range texts {
		if roll.Entry.Kind == "status" && strings.HasPrefix(text, "duplicate ") {
			_, amount, ok := strings.Cut(text, " status → $")
			if !ok {
				return "", "", 0, errors.New("invalid stored status conversion")
			}
			cash, err := storedCaseCash(amount)
			return "USD", "payout", cash, err
		}
		if roll.Entry.Kind == "fee" && strings.HasSuffix(text, " added as cash (voucher slots full)") {
			amount, _, ok := strings.Cut(strings.TrimPrefix(text, "$"), " fee · ")
			if !ok || !strings.HasPrefix(text, "$") {
				return "", "", 0, errors.New("invalid stored fee conversion")
			}
			cash, err := storedCaseCash(amount)
			return "USD", "payout", cash, err
		}
	}
	if roll.Note != "" {
		return "", "", 0, fmt.Errorf("unknown stored case note %q", roll.Note)
	}
	asset, kind, amount, err := battlepassRewardAmount(battlepass_sql.BPReward{
		Kind: roll.Entry.Kind, Value: roll.Entry.Value, Voucher: roll.Entry.Voucher,
		Cosmetic: roll.Entry.Cosmetic, CosmeticKind: roll.Entry.CosmeticKind,
	})
	if kind == "reward" {
		kind = "payout"
	}
	return asset, kind, amount, err
}

func loadCaseActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT c.id, c.rarity, c.roll, c.opened_at, c.source_level, c.source_track, s.label
		FROM bp_case_tokens c JOIN bp_seasons s ON s.id = c.season_id
		WHERE c.user_id = $1 AND c.opened_at IS NOT NULL;
	`, func(rows pgx.Rows) error {
		var id int64
		var rarity, track, season string
		var raw []byte
		var at time.Time
		var level int
		if err := rows.Scan(&id, &rarity, &raw, &at, &level, &track, &season); err != nil {
			return err
		}
		var roll battlepass_sql.BPCaseRoll
		if err := json.Unmarshal(raw, &roll); err != nil {
			return err
		}
		asset, kind, amount, err := caseActivityAmount(roll)
		if err != nil {
			return err
		}
		facts := []activityFact{activityFactFor("Season", season), activityFactFor("Rarity", rarity), activityFactFor("Level", strconv.Itoa(level)),
			activityFactFor("Track", track), activityFactFor("Granted", roll.Granted)}
		if roll.Entry.Kind == "cosmetic" {
			cosmeticID := roll.Entry.Cosmetic
			if cosmeticID == "" {
				cosmeticID = roll.Cosmetic
			}
			facts = append(facts, activityFactFor("Cosmetic", cosmeticID),
				activityFactFor("Reward", battlepass_sql.BPFormatReward(battlepass_sql.BPReward{
					Kind: roll.Entry.Kind, Cosmetic: cosmeticID,
					Display: roll.Entry.Display,
				})))
		}
		if roll.Note != "" {
			facts = append(facts, activityFactFor("Note", roll.Note))
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("bp:case:%d:open", id), "cases", kind,
			strings.Title(rarity)+" case", asset, amount, at, facts...))
		return nil
	})
}

func loadQuestActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT q.id, q.title, q.xp, q.claimed_at, q.category, q.period_key, s.label
		FROM bp_quests q JOIN bp_seasons s ON s.id = q.season_id
		WHERE q.user_id = $1 AND q.claimed_at IS NOT NULL;
	`, func(rows pgx.Rows) error {
		var id int64
		var title, category, period, season string
		var xp int
		var at time.Time
		if err := rows.Scan(&id, &title, &xp, &at, &category, &period, &season); err != nil {
			return err
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("bp:quest:%d", id), "quests", "xp", title,
			"XP", math.Abs(float64(xp)), at, activityFactFor("Season", season), activityFactFor("Category", category),
			activityFactFor("Period", period), activityFactFor("Quest ID", strconv.FormatInt(id, 10))))
		return nil
	})
}

func loadTicketActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	if err := activityRows(ctx, tx, userID, `
		SELECT p.id, p.rarity, p.price::float8, p.created_at,
			COALESCE(SUM(t.value) FILTER (WHERE t.revealed), 0)::float8,
			COUNT(t.id) FILTER (WHERE t.revealed), COUNT(t.id),
			MAX(t.revealed_at) FILTER (WHERE t.revealed)
		FROM ticket_packs p LEFT JOIN tickets t ON t.pack_id = p.id AND t.user_id = p.user_id
		WHERE p.user_id = $1 GROUP BY p.id ORDER BY p.created_at DESC, p.id DESC;
	`, func(rows pgx.Rows) error {
		var id int64
		var pack activityTicketPack
		if err := rows.Scan(&id, &pack.Rarity, &pack.Cost, &pack.PurchasedAt, &pack.Won, &pack.Opened, &pack.Total, &pack.LastOpenedAt); err != nil {
			return err
		}
		pack.ID = strconv.FormatInt(id, 10)
		pack.PurchasedAt = pack.PurchasedAt.UTC()
		if pack.LastOpenedAt != nil {
			at := pack.LastOpenedAt.UTC()
			pack.LastOpenedAt = &at
		}
		response.TicketPacks = append(response.TicketPacks, pack)
		response.Events = append(response.Events, makeActivityEvent("ticket:pack:"+pack.ID+":purchase", "tickets", "purchase",
			pack.Rarity+" ticket pack", "USD", -math.Abs(pack.Cost), pack.PurchasedAt,
			activityFactFor("Pack ID", pack.ID), activityFactFor("Rarity", pack.Rarity), activityFactFor("Game", "tickets")))
		return nil
	}); err != nil {
		return err
	}
	return activityRows(ctx, tx, userID, `
		SELECT id, pack_id, rarity, value::float8, revealed_at, idx
		FROM tickets WHERE user_id = $1 AND revealed;
	`, func(rows pgx.Rows) error {
		var id, packID int64
		var rarity string
		var value float64
		var at time.Time
		var index int
		if err := rows.Scan(&id, &packID, &rarity, &value, &at, &index); err != nil {
			return err
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("ticket:%d:payout", id), "tickets", "payout",
			rarity+" ticket payout", "USD", math.Abs(value), at, activityFactFor("Pack ID", strconv.FormatInt(packID, 10)),
			activityFactFor("Ticket", strconv.Itoa(index+1)), activityFactFor("Ticket ID", strconv.FormatInt(id, 10))))
		return nil
	})
}

func betActivityEvents(game string, id, roundID int64, amount float64, status string, payout *float64, createdAt time.Time, endedAt *time.Time) []activityEvent {
	facts := []activityFact{activityFactFor("Game", game), activityFactFor("Round", strconv.FormatInt(roundID, 10)), activityFactFor("Status", status)}
	prefix := fmt.Sprintf("game:%s:%d", game, id)
	events := []activityEvent{makeActivityEvent(prefix+":bet", "games", "bet", game+" bet", "USD", -math.Abs(amount), createdAt, facts...)}
	settled := (game == "rocket" && status == "cashed_out") || (game == "flip" && status == "won") ||
		(game == "pixel" && (status == "cashed_out" || status == "expired"))
	if settled && payout != nil && *payout > 0 && endedAt != nil {
		events = append(events, makeActivityEvent(prefix+":payout", "games", "payout", game+" payout", "USD", *payout, *endedAt, facts...))
	}
	return events
}

func loadBetActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT 'rocket', b.id, b.round_id, b.amount::float8, b.status, b.payout::float8, b.created_at, r.ended_at
		FROM rocket_bets b JOIN rocket_rounds r ON r.id = b.round_id WHERE b.user_id = $1
		UNION ALL
		SELECT 'flip', b.id, b.round_id, b.amount::float8, b.status, b.payout::float8, b.created_at, r.ended_at
		FROM flip_bets b JOIN flip_rounds r ON r.id = b.round_id WHERE b.user_id = $1
		UNION ALL
		SELECT 'pixel', id, round_id, amount::float8, status, payout::float8, created_at, ended_at
		FROM pixel_boards WHERE user_id = $1;
	`, func(rows pgx.Rows) error {
		var game, status string
		var id, roundID int64
		var amount float64
		var payout *float64
		var createdAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(&game, &id, &roundID, &amount, &status, &payout, &createdAt, &endedAt); err != nil {
			return err
		}
		response.Events = append(response.Events, betActivityEvents(game, id, roundID, amount, status, payout, createdAt, endedAt)...)
		return nil
	})
}

func loadSlotsActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT day, winnings, spins, wins FROM slots_daily WHERE user_id = $1;
	`, func(rows pgx.Rows) error {
		var day time.Time
		var winnings float64
		var spins, wins int
		if err := rows.Scan(&day, &winnings, &spins, &wins); err != nil {
			return err
		}
		date := day.Format("2006-01-02")
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("game:slots:daily:%d:%s", userID, date), "games",
			"payout", "Slots daily payouts", "USD", winnings, day,
			activityFactFor("Game", "slots"), activityFactFor("Date", date), activityFactFor("Granularity", "day"),
			activityFactFor("Spins", strconv.Itoa(spins)), activityFactFor("Wins", strconv.Itoa(wins))))
		return nil
	})
}

func marketActivityEvent(market string, id int64, symbol, operation string, amount, quantity, price float64, pnl *float64, at time.Time) (activityEvent, error) {
	switch operation {
	case "buy":
		amount = -math.Abs(amount)
	case "sell":
		amount = math.Abs(amount)
	default:
		return activityEvent{}, fmt.Errorf("unknown market operation %q", operation)
	}
	facts := []activityFact{activityFactFor("Market", market), activityFactFor("Symbol", symbol),
		activityFactFor("Quantity", activityNumber(quantity)), activityFactFor("Price", activityNumber(price))}
	if pnl != nil {
		facts = append(facts, activityFactFor("Realized P&L", activityNumber(*pnl)))
	}
	return makeActivityEvent(fmt.Sprintf("market:%s:%d", market, id), "markets", operation, symbol+" · "+operation,
		"USD", amount, at, facts...), nil
}

func loadMarketActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT 'stocks', id, symbol, operation_type, usd_amount::float8, quantity::float8, price::float8, realized_pnl::float8, created_at
		FROM stock_history WHERE user_id = $1
		UNION ALL
		SELECT 'commodities', id, symbol, operation_type, usd_amount::float8, quantity::float8, price::float8, realized_pnl::float8, created_at
		FROM commodity_history WHERE user_id = $1;
	`, func(rows pgx.Rows) error {
		var market, symbol, operation string
		var id int64
		var amount, quantity, price float64
		var pnl *float64
		var at time.Time
		if err := rows.Scan(&market, &id, &symbol, &operation, &amount, &quantity, &price, &pnl, &at); err != nil {
			return err
		}
		event, err := marketActivityEvent(market, id, symbol, operation, amount, quantity, price, pnl, at)
		if err != nil {
			return err
		}
		response.Events = append(response.Events, event)
		return nil
	})
}

func voucherActivitySource(source string) string {
	switch source {
	case "daily_rewards":
		return "daily"
	case "battlepass":
		return "battlepass"
	default:
		return "promo"
	}
}

func voucherActivityAsset(voucherType string) (string, error) {
	switch voucherType {
	case "usdt_credit":
		return "USD", nil
	case "lavx_credit":
		return "LAVX", nil
	case "ref_xp_credit":
		return "REF XP", nil
	default:
		return "", fmt.Errorf("unknown credit voucher %q", voucherType)
	}
}

func loadVoucherActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	return activityRows(ctx, tx, userID, `
		SELECT id, voucher_type, amount::float8, source, claimed_at FROM voucher_claims_log
		WHERE user_id = $1 AND voucher_type IN ('usdt_credit', 'lavx_credit', 'ref_xp_credit');
	`, func(rows pgx.Rows) error {
		var id int64
		var voucherType, source string
		var amount float64
		var at time.Time
		if err := rows.Scan(&id, &voucherType, &amount, &source, &at); err != nil {
			return err
		}
		asset, err := voucherActivityAsset(voucherType)
		if err != nil {
			return err
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("voucher:claim:%d", id), voucherActivitySource(source),
			"voucher_claim", "Voucher claimed · "+asset, asset, math.Abs(amount), at,
			activityFactFor("Origin", source), activityFactFor("Voucher", voucherType)))
		return nil
	})
}

// NFT из наград BP и ежедневок остаются в своих разделах, остальное — в разделе NFT.
func nftActivitySource(source string) string {
	switch source {
	case "Battle Pass":
		return "battlepass"
	case "Daily Rewards":
		return "daily"
	default:
		return "nft"
	}
}

func nftSaleMarket(kind string) string {
	switch kind {
	case "market":
		return "Player market"
	case "game":
		return "xwallet Market"
	default:
		return "Instant sale"
	}
}

func loadNftActivity(ctx context.Context, tx pgx.Tx, userID int, response *activityResponse) error {
	// Только реальные минты: покупки с маркета приходят из nft_sales и не дублируются.
	if err := activityRows(ctx, tx, userID, `
		SELECT i.id, COALESCE(t.name, i.template_id), COALESCE(t.rarity, ''), i.serial, i.source,
		       i.minted_at, COALESCE(t.base_price, 0)::float8
		FROM nft_items i LEFT JOIN nft_templates t ON t.id = i.template_id
		WHERE i.owner_id = $1 AND i.source NOT IN ('Market', 'xwallet Market');
	`, func(rows pgx.Rows) error {
		var id int64
		var name, rarity, source string
		var serial int
		var at time.Time
		var value float64
		if err := rows.Scan(&id, &name, &rarity, &serial, &source, &at, &value); err != nil {
			return err
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("nft:item:%d", id), nftActivitySource(source), "nft",
			fmt.Sprintf("%s #%d", name, serial), "NFT", 1, at,
			activityFactFor("Source", source), activityFactFor("Rarity", rarity), activityFactFor("Value", fmt.Sprintf("$%.2f", value))))
		return nil
	}); err != nil {
		return err
	}
	if err := activityRows(ctx, tx, userID, `
		SELECT s.id, COALESCE(t.name, s.template_id), COALESCE(t.rarity, ''), s.serial, s.kind,
		       s.price::float8, s.created_at,
		       CASE WHEN s.buyer_id = $1 THEN 'buy' ELSE 'sell' END
		FROM nft_sales s LEFT JOIN nft_templates t ON t.id = s.template_id
		WHERE s.buyer_id = $1 OR s.seller_id = $1;
	`, func(rows pgx.Rows) error {
		var id int64
		var name, rarity, kind, side string
		var serial int
		var price float64
		var at time.Time
		if err := rows.Scan(&id, &name, &rarity, &serial, &kind, &price, &at, &side); err != nil {
			return err
		}
		title := fmt.Sprintf("%s #%d", name, serial)
		market := nftSaleMarket(kind)
		if side == "buy" {
			response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("nft:sale:%d:buy", id), "nft", "buy",
				"Bought "+title, "USD", -math.Abs(price), at,
				activityFactFor("Market", market), activityFactFor("Price", fmt.Sprintf("$%.2f", price)),
				activityFactFor("Rarity", rarity), activityFactFor("Sale ID", strconv.FormatInt(id, 10))))
			return nil
		}
		net := price
		facts := []activityFact{activityFactFor("Market", market), activityFactFor("Gross price", fmt.Sprintf("$%.2f", price))}
		if kind == "market" {
			net = math.Floor(price*(1-nft_sql.MarketFee)*100) / 100
			facts = append(facts, activityFactFor("Fee", fmt.Sprintf("$%.2f", price-net)))
		}
		facts = append(facts, activityFactFor("Rarity", rarity), activityFactFor("Sale ID", strconv.FormatInt(id, 10)))
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("nft:sale:%d:sell", id), "nft", "sell",
			"Sold "+title, "USD", math.Abs(net), at, facts...))
		return nil
	}); err != nil {
		return err
	}
	if err := activityRows(ctx, tx, userID, `
		SELECT u.id, COALESCE(st.name, u.source_template_id), u.source_serial, u.source_value::float8,
		       COALESCE(tt.name, u.target_template_id), u.target_rarity, u.target_value::float8,
		       u.chance, u.won, u.created_at
		FROM nft_upgrades u
		LEFT JOIN nft_templates st ON st.id = u.source_template_id
		LEFT JOIN nft_templates tt ON tt.id = u.target_template_id
		WHERE u.user_id = $1;
	`, func(rows pgx.Rows) error {
		var id int64
		var fromName, toName, toRarity string
		var fromSerial int
		var fromValue, toValue, chance float64
		var won bool
		var at time.Time
		if err := rows.Scan(&id, &fromName, &fromSerial, &fromValue, &toName, &toRarity, &toValue, &chance, &won, &at); err != nil {
			return err
		}
		from := fmt.Sprintf("%s #%d", fromName, fromSerial)
		title := "Lost " + from + " upgrading to " + toName
		if won {
			title = "Upgraded " + from + " to " + toName
		}
		result := "Lost"
		if won {
			result = "Won"
		}
		response.Events = append(response.Events, makeActivityEvent(fmt.Sprintf("nft:upgrade:%d", id), "nft", "upgrade",
			title, "NFT", 0, at,
			activityFactFor("Result", result),
			activityFactFor("From", fmt.Sprintf("%s · $%.2f", from, fromValue)),
			activityFactFor("To", fmt.Sprintf("%s · $%.2f", toName, toValue)),
			activityFactFor("Rarity", toRarity),
			activityFactFor("Chance", fmt.Sprintf("%.1f%%", chance*100))))
		return nil
	}); err != nil {
		return err
	}
	return nil
}
