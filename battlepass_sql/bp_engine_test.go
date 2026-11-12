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
	duplicateCosmetic bool
	cosmeticErr       error
	voucherCount      int
	calls             []bpRewardTestCall
}

func (tx *bpRewardTestTx) record(query string, args []any) error {
	tx.calls = append(tx.calls, bpRewardTestCall{query: query, args: args})
	for _, arg := range args {
		if id, ok := arg.(string); ok && strings.HasPrefix(id, "card-") {
			return tx.cosmeticErr
		}
	}
	return nil
}

func (tx *bpRewardTestTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if err := tx.record(query, args); err != nil {
		return pgconn.CommandTag{}, err
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
	if strings.Contains(query, "COUNT(*) FROM user_vouchers") {
		return bpRewardTestCountRow{count: tx.voucherCount}
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

type bpRewardTestCountRow struct {
	count int
}

func (row bpRewardTestCountRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("unexpected voucher count scan %d", len(dest))
	}
	count, ok := dest[0].(*int)
	if !ok {
		return fmt.Errorf("unexpected voucher count scan %T", dest[0])
	}
	*count = row.count
	return nil
}

func (tx *bpRewardTestTx) grantWithArgs(id, source string) bool {
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

func (tx *bpRewardTestTx) cosmeticGrant(id, source string) bool {
	return tx.grantWithArgs(id, source)
}

func (tx *bpRewardTestTx) voucherGrant(voucherType, source string) bool {
	return tx.grantWithArgs(voucherType, source)
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

func TestBPDirectCosmeticsNeverConvertToCash(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		for _, fromCase := range []bool{false, true} {
			tx := &bpRewardTestTx{duplicateCosmetic: duplicate}
			_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCosmetic("card-cocoa"), 10, "free", fromCase)
			if err != nil || note != "" || len(tx.cashUpdates()) != 0 {
				t.Fatalf("cosmetic duplicate changed notes/cash: note=%q cash=%v err=%v", note, tx.cashUpdates(), err)
			}
			source := "battlepass"
			if fromCase {
				source = "case"
			}
			if !tx.cosmeticGrant("card-cocoa", source) {
				t.Fatalf("missing direct grant with source %s", source)
			}
		}
	}
	for _, reward := range []BPReward{
		{Kind: "cosmetic", Cosmetic: "not-a-cosmetic"},
		{Kind: "cosmetic", Cosmetic: "card-xlavelia"},
		{Kind: "cosmetic", Cosmetic: "card-wave"},
		{Kind: "cosmetic", Cosmetic: "card-cocoa", Value: 90},
	} {
		tx := &bpRewardTestTx{}
		if _, _, err := bpGrantRewardTx(context.Background(), tx, 7, 1, reward, 10, "free", false); err == nil || len(tx.calls) != 0 {
			t.Fatalf("invalid/admin/cash cosmetic reward accepted: %+v, %v", reward, err)
		}
	}
}

func TestBPCustomVoucherGrant(t *testing.T) {
	for _, voucher := range []string{"custom_free", "custom_20", "custom_50"} {
		for _, fromCase := range []bool{false, true} {
			tx := &bpRewardTestTx{}
			_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCustom(voucher), 25, "free", fromCase)
			if err != nil || note != "" {
				t.Fatalf("custom voucher grant: %v %q", err, note)
			}
			source := "battlepass"
			if fromCase {
				source = "case"
			}
			if !tx.voucherGrant(voucher, source) {
				t.Fatalf("missing custom voucher grant %s from %s", voucher, source)
			}
			if len(tx.cashUpdates()) != 0 {
				t.Fatal("custom voucher paid cash")
			}
		}
	}
}

func TestBPCustomVoucherSlotsFullConvertsToCash(t *testing.T) {
	tx := &bpRewardTestTx{voucherCount: BPVoucherSlots}
	_, note, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCustom("custom_50"), 25, "free", true)
	if err != nil || !strings.Contains(note, "added as cash") || len(tx.cashUpdates()) != 1 || tx.cashUpdates()[0] != BPDuplicateStatusCash {
		t.Fatalf("slots-full conversion: %v %q %v", err, note, tx.cashUpdates())
	}
	tx = &bpRewardTestTx{voucherCount: BPVoucherSlots}
	if _, _, err := bpGrantRewardTx(context.Background(), tx, 7, 1, bpCustom("custom_50"), 25, "free", false); !errors.Is(err, BPErrVoucherSlotsFull) {
		t.Fatalf("slots-full without conversion: %v", err)
	}
}

func TestBPCosmeticFailurePreventsDuplicateConversion(t *testing.T) {
	failure := errors.New("cosmetic grant failed")
	for _, reward := range []BPReward{bpCosmetic("card-supernova"), bpCosmetic("card-cocoa")} {
		tx := &bpRewardTestTx{duplicateCosmetic: true, cosmeticErr: failure}
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
