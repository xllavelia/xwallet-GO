package battlepass_sql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type bpRewardTestCall struct {
	query string
	args  []any
}

type bpRewardTestTx struct {
	pgx.Tx
	duplicateStatus   bool
	duplicateCosmetic bool
	cosmeticErr       error
	calls             []bpRewardTestCall
}

func (tx *bpRewardTestTx) record(query string, args []any) error {
	tx.calls = append(tx.calls, bpRewardTestCall{query: query, args: args})
	for _, arg := range args {
		if id, ok := arg.(string); ok && (strings.HasPrefix(id, "theme-") || strings.HasPrefix(id, "emblem-") || strings.HasPrefix(id, "pattern-")) {
			return tx.cosmeticErr
		}
	}
	return nil
}

func (tx *bpRewardTestTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if err := tx.record(query, args); err != nil {
		return pgconn.CommandTag{}, err
	}
	if strings.Contains(query, "INSERT INTO user_statuses") {
		if tx.duplicateStatus {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}
	if strings.Contains(query, "UPDATE wallets") {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	if tx.duplicateCosmetic {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *bpRewardTestTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	if err := tx.record(query, args); err != nil {
		return bpRewardTestRow{err: err}
	}
	if !strings.Contains(query, "FROM users u WHERE u.id=$1") {
		return bpRewardTestRow{err: fmt.Errorf("unexpected cosmetic grant query %q", query)}
	}
	return bpRewardTestRow{}
}

type bpRewardTestRow struct {
	err error
}

func (row bpRewardTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 1 {
		return fmt.Errorf("unexpected cosmetic grant scan count %d", len(dest))
	}
	banned, ok := dest[0].(*bool)
	if !ok {
		return fmt.Errorf("unexpected cosmetic grant scan %T", dest[0])
	}
	*banned = false
	return nil
}

func (tx *bpRewardTestTx) cosmeticGrant(id, source string) bool {
	for _, call := range tx.calls {
		var hasID, hasSource bool
		for _, arg := range call.args {
			if value, ok := arg.(string); ok {
				hasID = hasID || value == id
				hasSource = hasSource || value == source
			}
		}
		if hasID && hasSource {
			return true
		}
	}
	return false
}

func (tx *bpRewardTestTx) cashUpdates() []float64 {
	var amounts []float64
	for _, call := range tx.calls {
		if strings.Contains(call.query, "UPDATE wallets") {
			if amount, ok := call.args[0].(float64); ok {
				amounts = append(amounts, amount)
			}
		}
	}
	return amounts
}

func TestBPStatusGrantKeepsDuplicateCashAndGrantsCosmetic(t *testing.T) {
	for _, test := range []struct {
		status, id, legacyLabel string
		fallback                float64
	}{
		{"lucky", "theme-sage", "Lucky", 25},
		{"valter", "theme-dusk", "Valter", 100},
		{"chicken_burger", "emblem-fox", "Chicken Burger", 15},
		{"silver", "theme-silver", "Silver", 20},
		{"gold", "theme-gold", "Gold", 50},
		{"platinum", "theme-platinum", "Platinum", 150},
		{"legend", "emblem-dragon", "Legend", 500},
		{"owner", "theme-supernova", "Owner", 2000},
		{"first", "emblem-crown", "First", 1000},
	} {
		for _, duplicateStatus := range []bool{false, true} {
			for _, duplicateCosmetic := range []bool{false, true} {
				for _, fromCase := range []bool{false, true} {
					tx := &bpRewardTestTx{duplicateStatus: duplicateStatus, duplicateCosmetic: duplicateCosmetic}
					reward := bpStatus(test.status)
					// Stored Status is authoritative; a supplied cosmetic cannot turn
					// the historical owner drop into an administrative cosmetic.
					reward.Cosmetic = "theme-xlavelia"
					_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, reward, 25, "free", fromCase)
					if err != nil {
						t.Fatal(err)
					}
					source := "battlepass"
					if fromCase {
						source = "case"
					}
					if !tx.cosmeticGrant(test.id, source) {
						t.Fatalf("missing mapped cosmetic grant for %s (status duplicate %v, cosmetic duplicate %v)", test.status, duplicateStatus, duplicateCosmetic)
					}
					cash := tx.cashUpdates()
					if duplicateStatus {
						wantNote := fmt.Sprintf("duplicate %s status → $%s", test.legacyLabel, bpNum(test.fallback))
						if len(cash) != 1 || cash[0] != test.fallback || note != wantNote {
							t.Fatalf("legacy duplicate behavior changed for %s: cash=%v note=%q", test.status, cash, note)
						}
					} else if len(cash) != 0 || note != "" {
						t.Fatalf("first status converted to cash: %v/%s", cash, note)
					}
				}
			}
		}
	}
	// Historical rewards with no fallback value still use the old $50 default.
	tx := &bpRewardTestTx{duplicateStatus: true}
	_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, BPReward{Kind: "status", Status: "gold"}, 25, "free", false)
	if err != nil || note != "duplicate Gold status → $50" || len(tx.cashUpdates()) != 1 || tx.cashUpdates()[0] != BPDuplicateStatusCash {
		t.Fatalf("default fallback changed: %s, %v", note, err)
	}
}

func TestBPDirectCosmeticsNeverConvertToCash(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		for _, fromCase := range []bool{false, true} {
			tx := &bpRewardTestTx{duplicateCosmetic: duplicate}
			_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCosmetic("pattern-waves"), 10, "free", fromCase)
			if err != nil || note != "" || len(tx.cashUpdates()) != 0 {
				t.Fatalf("cosmetic duplicate changed notes/cash: note=%q cash=%v err=%v", note, tx.cashUpdates(), err)
			}
			source := "battlepass"
			if fromCase {
				source = "case"
			}
			if !tx.cosmeticGrant("pattern-waves", source) {
				t.Fatalf("missing direct grant with source %s", source)
			}
		}
	}
	for _, reward := range []BPReward{
		{Kind: "cosmetic", Cosmetic: "not-a-cosmetic"},
		{Kind: "cosmetic", Cosmetic: "theme-xlavelia"},
		{Kind: "cosmetic", Cosmetic: "emblem-sigil"},
		{Kind: "cosmetic", Cosmetic: "pattern-monogram"},
		{Kind: "cosmetic", Cosmetic: "pattern-waves", Value: 90},
	} {
		tx := &bpRewardTestTx{}
		if _, _, err := bpGrantRewardTx(context.Background(), tx, 7, 1, reward, 10, "free", false); err == nil || len(tx.calls) != 0 {
			t.Fatalf("invalid/admin/cash cosmetic reward accepted: %+v, %v", reward, err)
		}
	}
}

func TestBPCosmeticFailurePreventsDuplicateConversion(t *testing.T) {
	failure := errors.New("cosmetic grant failed")
	for _, reward := range []BPReward{bpStatus("owner"), bpCosmetic("pattern-waves")} {
		tx := &bpRewardTestTx{duplicateStatus: true, cosmeticErr: failure}
		_, _, err := bpGrantRewardTx(context.Background(), tx, 7, 1, reward, 10, "pass", true)
		if !errors.Is(err, failure) || len(tx.cashUpdates()) != 0 {
			t.Fatalf("grant error not propagated before cash conversion: %v, %v", err, tx.cashUpdates())
		}
	}
}

func TestBPCash90GrantUnchanged(t *testing.T) {
	tx := &bpRewardTestTx{}
	_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCash(90), 39, "free", false)
	cash := tx.cashUpdates()
	if err != nil || note != "" || len(cash) != 1 || cash[0] != 90 || len(tx.calls) != 1 {
		t.Fatalf("$90 reward changed: cash=%v note=%s err=%v", cash, note, err)
	}
}

func TestBPRollCaseReturnsCatalogEntry(t *testing.T) {
	for _, rarity := range BPCaseOrder {
		def := BPCases[rarity]
		entry, index, err := bpRollCase(def)
		if err != nil || index < 0 || index >= len(def.Entries) || entry != def.Entries[index] {
			t.Fatalf("invalid %s case selection: %+v/%d, %v", rarity, entry, index, err)
		}
	}
}
