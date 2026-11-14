package home_http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"xwallet-server/battlepass_sql"

	"github.com/jackc/pgx/v5"
)

func TestDailyActivityComponents(t *testing.T) {
	at := time.Date(2026, 11, 2, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		track   string
		day     int
		assets  []string
		amounts []float64
	}{
		{"start", 1, []string{"USD"}, []float64{10}},
		{"start", 5, []string{"voucher"}, []float64{1}},
		{"start", 2, []string{"XP"}, []float64{10}},
		{"start", 4, []string{"REF XP"}, []float64{5}},
		{"start", 30, []string{"USD", "LAVX"}, []float64{25, 5}},
		{"pro", 30, []string{"USD", "LAVX", "voucher", "XP"}, []float64{50, 10, 5, 700}},
	} {
		events, err := dailyActivityEvents(71, test.track, test.day, at)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != len(test.assets) {
			t.Fatalf("%s/%d: got %d events", test.track, test.day, len(events))
		}
		ids := map[string]bool{}
		for i, event := range events {
			if event.Asset != test.assets[i] || event.Amount != test.amounts[i] || event.Source != "daily" || !event.At.Equal(at) {
				t.Fatalf("%s/%d: unexpected event %+v", test.track, test.day, event)
			}
			if ids[event.ID] || !strings.HasPrefix(event.ID, "daily:71:") {
				t.Fatalf("duplicate or unstable ID %s", event.ID)
			}
			ids[event.ID] = true
			if event.Asset == "voucher" && event.Kind != "voucher_issued" {
				t.Fatalf("voucher issuance treated as income: %+v", event)
			}
		}
	}
	for _, test := range []struct {
		track string
		day   int
	}{{"unknown", 1}, {"start", 0}, {"pro", 31}} {
		if _, err := dailyActivityEvents(1, test.track, test.day, at); err == nil {
			t.Fatalf("expected unknown reward error for %+v", test)
		}
	}
}

func TestBattlepassActualCashComponents(t *testing.T) {
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		level int
		track string
		cash  float64
	}{{39, "free", 90}, {27, "pass", 90}, {25, "free", 10}, {25, "pass", 20}, {40, "pass", 0}, {50, "pass", 0}} {
		events, err := battlepassActivityEvents(5, 2, "Season 2", test.level, test.track, at)
		if err != nil {
			t.Fatal(err)
		}
		var cash float64
		for _, event := range events {
			if event.Source != "battlepass" || !event.At.Equal(at) {
				t.Fatalf("wrong source/date: %+v", event)
			}
			if event.Asset == "USD" {
				cash += event.Amount
			}
			if event.Asset == "cosmetic" && (event.Amount != 1 || event.Kind != "cosmetic") {
				t.Fatalf("status fallback value used as amount: %+v", event)
			}
			if event.Asset == "case" && (event.Amount != 1 || event.Kind != "case_issued") {
				t.Fatalf("case issuance treated as payout: %+v", event)
			}
		}
		if cash != test.cash {
			t.Fatalf("level %d/%s: cash = %v, want %v", test.level, test.track, cash, test.cash)
		}
	}
	first, _ := battlepassActivityEvents(5, 2, "Season 2", 39, "free", at)
	otherUser, _ := battlepassActivityEvents(6, 2, "Season 2", 39, "free", at)
	otherSeason, _ := battlepassActivityEvents(5, 3, "Season 3", 39, "free", at)
	otherTrack, _ := battlepassActivityEvents(5, 2, "Season 2", 39, "pass", at)
	if first[0].ID == otherUser[0].ID || first[0].ID == otherSeason[0].ID || first[0].ID == otherTrack[0].ID {
		t.Fatal("claim namespaces collide")
	}
	for _, test := range []struct {
		level int
		track string
	}{{0, "free"}, {51, "free"}, {1, "unknown"}} {
		if _, err := battlepassActivityEvents(1, 1, "Season", test.level, test.track, at); err == nil {
			t.Fatal("unknown battle pass reward did not fail")
		}
	}
}

func TestBattlepassHistoryDoesNotInventCosmetics(t *testing.T) {
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		level int
		track string
	}{
		{5, "free"}, {10, "free"}, {12, "free"}, {15, "free"}, {20, "free"},
		{8, "pass"}, {10, "pass"}, {14, "pass"}, {18, "pass"}, {32, "pass"}, {36, "pass"}, {45, "pass"},
	} {
		events, err := battlepassActivityEvents(5, 2, "Season 2", test.level, test.track, at)
		if err != nil {
			t.Fatal(err)
		}
		row := battlepass_sql.BPLevelRewardsFor(test.level)
		rewards := row.Free
		if test.track == "pass" {
			rewards = row.Pass
		}
		var original []battlepass_sql.BPReward
		for _, reward := range rewards {
			if reward.Kind != "cosmetic" && reward.Kind != "nft" {
				original = append(original, reward)
			}
		}
		if len(events) != len(original) {
			t.Fatalf("level %d/%s invented an old unlock: %+v", test.level, test.track, events)
		}
		for i, event := range events {
			asset, kind, amount, err := battlepassRewardAmount(original[i])
			if err != nil || event.Asset != asset || event.Kind != kind || event.Amount != amount || event.Asset == "cosmetic" {
				t.Fatalf("old reward changed: %+v, %v", event, err)
			}
			if event.ID != fmt.Sprintf("bp:claim:5:2:%d:%s:%d", test.level, test.track, i) {
				t.Fatalf("original event ID changed: %s", event.ID)
			}
		}
	}
}

func TestCaseUsesPersistedConversions(t *testing.T) {
	for _, test := range []struct {
		name   string
		roll   battlepass_sql.BPCaseRoll
		asset  string
		kind   string
		amount float64
	}{
		{"cash", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "cash", Value: 25}, Granted: "$25"}, "USD", "payout", 25},
		{"lavx", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "lavx", Value: 10}, Granted: "10 LAVX"}, "LAVX", "payout", 10},
		{"ref", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "ref", Value: 80}, Granted: "80 REF XP"}, "REF XP", "payout", 80},
		{"fee voucher", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "fee", Value: 100}, Granted: "$100 fee · 5d"}, "voucher", "voucher_issued", 100},
		{"status", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "status", Value: 2000}, Granted: "Owner status"}, "cosmetic", "cosmetic", 1},
		{"cosmetic", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "cosmetic", Cosmetic: "pattern-waves"}, Cosmetic: "pattern-waves", Granted: "Waves pattern"}, "cosmetic", "cosmetic", 1},
		{"cosmetic not cash", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "cosmetic", Cosmetic: "emblem-eclipse", Value: 2000}, Granted: "Eclipse emblem"}, "cosmetic", "cosmetic", 1},
		{"mapped duplicate note", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "status", Cosmetic: "theme-supernova"}, Note: "duplicate Owner status → $2000", Granted: "duplicate Owner status → $2000"}, "USD", "payout", 2000},
		{"duplicate note", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "status", Value: 2000}, Note: "duplicate Owner status → $17.5", Granted: "duplicate Owner status → $17.5"}, "USD", "payout", 17.5},
		{"duplicate granted", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "status"}, Granted: "duplicate Gold status → $42"}, "USD", "payout", 42},
		{"full slots note", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "fee", Value: 100}, Note: "$147.3 fee · 5d added as cash (voucher slots full)", Granted: "$147.3 fee · 5d added as cash (voucher slots full)"}, "USD", "payout", 147.3},
		{"full slots granted", battlepass_sql.BPCaseRoll{Entry: battlepass_sql.BPCaseEntry{Kind: "fee", Value: 20}, Granted: "$31 fee · 4d added as cash (voucher slots full)"}, "USD", "payout", 31},
	} {
		t.Run(test.name, func(t *testing.T) {
			asset, kind, amount, err := caseActivityAmount(test.roll)
			if err != nil {
				t.Fatal(err)
			}
			if asset != test.asset || kind != test.kind || amount != test.amount {
				t.Fatalf("got %s/%s/%v, want %s/%s/%v", asset, kind, amount, test.asset, test.kind, test.amount)
			}
		})
	}
	for _, roll := range []battlepass_sql.BPCaseRoll{
		{},
		{Entry: battlepass_sql.BPCaseEntry{Kind: "status"}, Note: "duplicate Owner status → $not-a-number"},
		{Entry: battlepass_sql.BPCaseEntry{Kind: "status"}, Note: "duplicate Owner status → $NaN"},
		{Entry: battlepass_sql.BPCaseEntry{Kind: "fee"}, Note: "$Inf fee · 5d added as cash (voucher slots full)"},
		{Entry: battlepass_sql.BPCaseEntry{Kind: "fee"}, Note: "unrecognized conversion"},
		{Entry: battlepass_sql.BPCaseEntry{Kind: "cosmetic", Cosmetic: "pattern-waves"}, Note: "duplicate Waves pattern → $25"},
	} {
		if _, _, _, err := caseActivityAmount(roll); err == nil {
			t.Fatalf("malformed stored roll silently accepted: %+v", roll)
		}
	}
}

func TestCaseCosmeticFactsUseStoredItem(t *testing.T) {
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		raw, id, asset string
		amount         float64
	}{
		{`{"entry":{"kind":"cosmetic","cosmetic":"pattern-waves","display":"Waves pattern"},"cosmetic":"pattern-waves","granted":"Waves pattern"}`, "pattern-waves", "cosmetic", 1},
		{`{"entry":{"kind":"cosmetic","display":"Eclipse emblem"},"cosmetic":"emblem-eclipse","granted":"Eclipse emblem"}`, "emblem-eclipse", "cosmetic", 1},
	} {
		tx := &activityFixtureTx{sets: [][][]any{{{int64(91), "rare", []byte(test.raw), at, 10, "free", "Season 1"}}}}
		response := newActivityResponse()
		if err := loadCaseActivity(context.Background(), tx, 5, &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Events) != 1 {
			t.Fatalf("case event count = %d", len(response.Events))
		}
		event := response.Events[0]
		facts := map[string]string{}
		for _, fact := range event.Facts {
			facts[fact.Label] = fact.Value
		}
		if event.Asset != test.asset || event.Amount != test.amount || facts["Cosmetic"] != test.id || facts["Reward"] == "" || strings.HasSuffix(facts["Reward"], " status") {
			t.Fatalf("stored case item/amount missing or invented: %+v", event)
		}
	}
}

func TestBetDatesAndPendingPayouts(t *testing.T) {
	created := time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)
	ended := time.Date(2026, 11, 1, 0, 1, 0, 0, time.UTC)
	payout := 19.0
	for _, test := range []struct {
		game   string
		status string
		end    *time.Time
		payout *float64
		count  int
	}{
		{"rocket", "cashed_out", &ended, &payout, 2},
		{"flip", "won", &ended, &payout, 2},
		{"pixel", "cashed_out", &ended, &payout, 2},
		{"pixel", "expired", &ended, &payout, 2},
		{"rocket", "active", &ended, &payout, 1},
		{"flip", "active", &ended, &payout, 1},
		{"pixel", "active", &ended, &payout, 1},
		{"rocket", "cashed_out", nil, &payout, 1},
		{"flip", "won", nil, &payout, 1},
		{"pixel", "lost", &ended, nil, 1},
	} {
		events := betActivityEvents(test.game, 3, 8, 10, test.status, test.payout, created, test.end)
		if len(events) != test.count || events[0].Amount != -10 || !events[0].At.Equal(created) {
			t.Fatalf("unexpected bet events for %+v: %+v", test, events)
		}
		if len(events) == 2 && (events[1].Amount != payout || !events[1].At.Equal(ended) || events[0].ID == events[1].ID) {
			t.Fatalf("payout uses wrong amount/date/ID: %+v", events)
		}
	}
}

func TestMarketAndVoucherConversions(t *testing.T) {
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	pnl := -4.0
	buy, err := marketActivityEvent("stocks", 1, "ABC", "buy", 90, 3, 30, nil, at)
	if err != nil || buy.Amount != -90 || buy.Asset != "USD" {
		t.Fatalf("wrong buy: %+v, %v", buy, err)
	}
	sell, err := marketActivityEvent("commodities", 1, "GOLD", "sell", 90, 3, 30, &pnl, at)
	if err != nil || sell.Amount != 90 || sell.ID == buy.ID || sell.Source != "markets" {
		t.Fatalf("wrong sell: %+v, %v", sell, err)
	}
	if _, err := marketActivityEvent("stocks", 1, "ABC", "unknown", 90, 3, 30, nil, at); err == nil {
		t.Fatal("unknown operation silently accepted")
	}
	for source, expected := range map[string]string{"daily_rewards": "daily", "battlepass": "battlepass", "welcome": "promo", "promo": "promo", "": "promo"} {
		if actual := voucherActivitySource(source); actual != expected {
			t.Fatalf("source %q = %q, want %q", source, actual, expected)
		}
	}
	for voucher, expected := range map[string]string{"usdt_credit": "USD", "lavx_credit": "LAVX", "ref_xp_credit": "REF XP"} {
		actual, err := voucherActivityAsset(voucher)
		if err != nil || actual != expected {
			t.Fatalf("voucher %q = %q, %v", voucher, actual, err)
		}
	}
	if _, err := voucherActivityAsset("fee_discount"); err == nil {
		t.Fatal("fee discount counted as a cash claim")
	}
}

func TestActivityJSONContract(t *testing.T) {
	response := newActivityResponse()
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 3 || string(decoded["events"]) != "[]" || string(decoded["ticketPacks"]) != "[]" || !strings.HasPrefix(string(decoded["coverage"]), "[") {
		t.Fatalf("wrong empty response contract: %s", body)
	}
	zone := time.FixedZone("offset", 3*60*60)
	at := time.Date(2026, 11, 2, 3, 0, 0, 0, zone)
	event := makeActivityEvent("test:1", "daily", "reward", "Reward", "USD", 10, at)
	body, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"at":"2026-11-02T00:00:00Z"`) || !strings.Contains(string(body), `"facts":[]`) {
		t.Fatalf("wrong event dates or facts: %s", body)
	}
	body, err = json.Marshal(activityTicketPack{ID: "9", PurchasedAt: at.UTC()})
	if err != nil || strings.Contains(string(body), "lastOpenedAt") {
		t.Fatalf("unopened pack has invented open date: %s, %v", body, err)
	}
	if !strings.Contains(string(decoded["coverage"]), "Slots purchase dates are not recorded; dated totals include payouts only.") {
		t.Fatal("missing slots coverage limitation")
	}
}

type activityTestTx struct {
	pgx.Tx
	rows pgx.Rows
	err  error
}

func (tx activityTestTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return tx.rows, tx.err
}

type activityTestRows struct {
	pgx.Rows
	remaining int
	scanErr   error
	rowsErr   error
	closed    bool
}

func (rows *activityTestRows) Next() bool {
	if rows.remaining == 0 {
		return false
	}
	rows.remaining--
	return true
}

func (rows *activityTestRows) Scan(...any) error { return rows.scanErr }
func (rows *activityTestRows) Err() error        { return rows.rowsErr }
func (rows *activityTestRows) Close()            { rows.closed = true }

func TestActivityRowsPropagatesEveryError(t *testing.T) {
	failure := errors.New("read failed")
	for _, test := range []struct {
		name     string
		queryErr error
		scanErr  error
		rowsErr  error
	}{
		{"query", failure, nil, nil},
		{"scan", nil, failure, nil},
		{"iteration", nil, nil, failure},
		{"success", nil, nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := &activityTestRows{remaining: 1, scanErr: test.scanErr, rowsErr: test.rowsErr}
			tx := activityTestTx{rows: rows, err: test.queryErr}
			err := activityRows(context.Background(), tx, 1, "SELECT", func(rows pgx.Rows) error { return rows.Scan() })
			want := test.queryErr != nil || test.scanErr != nil || test.rowsErr != nil
			if (err != nil) != want || (want && !errors.Is(err, failure)) {
				t.Fatalf("wrong error: %v", err)
			}
			if test.queryErr == nil && !rows.closed {
				t.Fatal("rows not closed")
			}
		})
	}
	response, err := loadActivity(context.Background(), activityTestTx{err: failure}, 1)
	if !errors.Is(err, failure) || !reflect.DeepEqual(response, activityResponse{}) {
		t.Fatalf("returned silent partial response: %+v, %v", response, err)
	}
}

type activityFixtureTx struct {
	pgx.Tx
	sets    [][][]any
	queries []string
}

func (tx *activityFixtureTx) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	tx.queries = append(tx.queries, query)
	if len(tx.sets) == 0 {
		return nil, errors.New("unexpected query")
	}
	rows := &activityFixtureRows{values: tx.sets[0]}
	tx.sets = tx.sets[1:]
	return rows, nil
}

type activityFixtureRows struct {
	pgx.Rows
	values [][]any
	next   int
}

func (rows *activityFixtureRows) Next() bool {
	if rows.next == len(rows.values) {
		return false
	}
	rows.next++
	return true
}

func (rows *activityFixtureRows) Scan(dest ...any) error {
	values := rows.values[rows.next-1]
	if len(dest) != len(values) {
		return errors.New("wrong scan column count")
	}
	for i, value := range values {
		target := reflect.ValueOf(dest[i]).Elem()
		if value == nil {
			target.SetZero()
		} else if reflect.TypeOf(value).AssignableTo(target.Type()) {
			target.Set(reflect.ValueOf(value))
		} else {
			return fmt.Errorf("wrong scan type at column %d", i)
		}
	}
	return nil
}

func (rows *activityFixtureRows) Err() error { return nil }
func (rows *activityFixtureRows) Close()     {}

func TestTicketPackConversionKeepsPurchasesAndRevealsSeparate(t *testing.T) {
	purchase := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	firstOpen := purchase.Add(24 * time.Hour)
	lastOpen := firstOpen.Add(24 * time.Hour)
	tx := &activityFixtureTx{sets: [][][]any{
		{
			{int64(9), "rare", float64(50), purchase, float64(8), 2, 5, &lastOpen},
			{int64(10), "rare", float64(50), lastOpen, float64(0), 0, 5, nil},
		},
		{
			{int64(41), int64(9), "rare", float64(3), firstOpen, 0},
			{int64(42), int64(9), "rare", float64(5), lastOpen, 1},
		},
	}}
	response := newActivityResponse()
	if err := loadTicketActivity(context.Background(), tx, 7, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.TicketPacks) != 2 || len(response.Events) != 4 {
		t.Fatalf("pack or event rows omitted: %+v", response)
	}
	pack := response.TicketPacks[0]
	if pack.ID != "9" || pack.Won != 8 || pack.Cost != 50 || pack.Opened != 2 || pack.Total != 5 || !pack.PurchasedAt.Equal(purchase) || !pack.LastOpenedAt.Equal(lastOpen) {
		t.Fatalf("wrong revealed pack aggregate: %+v", pack)
	}
	unopened := response.TicketPacks[1]
	if unopened.Won != 0 || unopened.Opened != 0 || unopened.LastOpenedAt != nil {
		t.Fatalf("unopened results or dates exposed: %+v", unopened)
	}
	var expense, income float64
	ids := map[string]bool{}
	for _, event := range response.Events {
		if event.Source != "tickets" || event.Asset != "USD" || ids[event.ID] {
			t.Fatalf("wrong ticket namespace/source/asset: %+v", event)
		}
		ids[event.ID] = true
		var packID string
		for _, fact := range event.Facts {
			if fact.Label == "Pack ID" {
				packID = fact.Value
			}
		}
		if packID != "9" && packID != "10" {
			t.Fatalf("ticket event missing pack link: %+v", event)
		}
		if event.Kind == "purchase" {
			expense -= event.Amount
			if packID == "9" && !event.At.Equal(purchase) {
				t.Fatal("purchase assigned a reveal date")
			}
		} else {
			income += event.Amount
			if packID != "9" || (!event.At.Equal(firstOpen) && !event.At.Equal(lastOpen)) {
				t.Fatal("payout assigned hidden results or purchase date")
			}
		}
	}
	if expense != 100 || income != 8 {
		t.Fatalf("wrong cash flow: expense=%v income=%v", expense, income)
	}
	if len(tx.queries) != 2 || !strings.Contains(tx.queries[0], "SUM(t.value) FILTER (WHERE t.revealed)") || !strings.Contains(tx.queries[1], "WHERE user_id = $1 AND revealed") {
		t.Fatal("ticket queries do not restrict winnings to revealed tickets")
	}
	for _, query := range tx.queries {
		if strings.Contains(strings.ToUpper(query), "LIMIT") {
			t.Fatal("ticket history silently truncated")
		}
	}
}

func TestSlotsDailyHistoryExceedsSpinRetention(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := [][]any{}
	for i := 0; i < 231; i++ {
		values = append(values, []any{start.AddDate(0, 0, i), float64(10), 20, 4})
	}
	tx := &activityFixtureTx{sets: [][][]any{values}}
	response := newActivityResponse()
	if err := loadSlotsActivity(context.Background(), tx, 7, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Events) != len(values) {
		t.Fatalf("daily totals truncated: got %d", len(response.Events))
	}
	for _, event := range response.Events {
		if event.Amount != 10 || event.Kind != "payout" || event.At.Hour() != 0 {
			t.Fatalf("invented slots purchase or time: %+v", event)
		}
	}
	if strings.Contains(strings.ToUpper(tx.queries[0]), "LIMIT") || strings.Contains(tx.queries[0], "slots_history") {
		t.Fatal("slots payouts rely on retained spin history")
	}
}

func TestActivityHandlerRejectsUnauthenticatedAndNonGET(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
	}{{http.MethodGet, http.StatusUnauthorized}, {http.MethodPost, http.StatusMethodNotAllowed}} {
		request := httptest.NewRequest(test.method, "/history/activity", nil)
		recorder := httptest.NewRecorder()
		GetActivityHandler(nil)(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("%s returned %d, want %d", test.method, recorder.Code, test.status)
		}
		if test.method != http.MethodGet && recorder.Header().Get("Allow") != http.MethodGet {
			t.Fatal("missing Allow header")
		}
	}
}
