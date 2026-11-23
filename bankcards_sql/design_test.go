package bankcards_sql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"xwallet-server/profile_sql"
)

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func photoURL(data []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
}

func jpegWithSize(t *testing.T, data []byte, size int) []byte {
	t.Helper()
	out := append([]byte(nil), data[:2]...)
	remaining := size - len(data)
	for remaining > 0 {
		segmentSize := min(remaining, 65537)
		if remaining-segmentSize > 0 && remaining-segmentSize < 4 {
			segmentSize -= 4
		}
		if segmentSize < 4 {
			t.Fatal("not enough room for JPEG comment")
		}
		segment := make([]byte, segmentSize)
		segment[0], segment[1] = 0xff, 0xfe
		binary.BigEndian.PutUint16(segment[2:4], uint16(segmentSize-2))
		out = append(out, segment...)
		remaining -= segmentSize
	}
	return append(out, data[2:]...)
}

func TestCustomPresetEligibility(t *testing.T) {
	eligible := map[string]bool{"card-sage": true, "card-coral": true, "card-glacier": true, "card-silk": true}
	owned := map[string]bool{}
	for _, design := range profile_sql.Catalog() {
		owned[design.ID] = true
	}
	for _, design := range profile_sql.Catalog() {
		for _, isAdmin := range []bool{false, true} {
			for _, ownership := range []map[string]bool{nil, owned} {
				choice, err := ValidateDesignChoice(DesignChoice{Custom: &CardCustomDesign{DesignKey: design.ID}}, ownership, isAdmin)
				if !eligible[design.ID] {
					if !errors.Is(err, ErrCustomInvalid) {
						t.Fatalf("ineligible custom %s admin=%v owned=%v: %v", design.ID, isAdmin, ownership != nil, err)
					}
					continue
				}
				if err != nil || choice.Custom == nil || choice.Custom.DesignKey != design.ID || choice.Custom.Photo != "" {
					t.Fatalf("eligible custom %s: %+v %v", design.ID, choice, err)
				}
			}
		}
	}
	input := &CardCustomDesign{DesignKey: " Card-Sage "}
	normalized, err := ValidateCustomDesign(input)
	if err != nil || normalized.DesignKey != "card-sage" || input.DesignKey != " Card-Sage " {
		t.Fatalf("normalization mutated input or failed: %+v %v", normalized, err)
	}
	for _, key := range []string{"unknown", "theme-gold", "0", " ", "card-nebula", "card-xlavelia"} {
		if _, err := ValidateCustomDesign(&CardCustomDesign{DesignKey: key}); !errors.Is(err, ErrCustomInvalid) {
			t.Fatalf("invalid custom key %q: %v", key, err)
		}
	}
}

func TestCustomPhotoValidation(t *testing.T) {
	data := testJPEG(t, 48, 24)
	url := photoURL(data)
	out, err := ValidateCustomDesign(&CardCustomDesign{Photo: url})
	if err != nil || out.Photo != url || out.DesignKey != "" {
		t.Fatalf("valid JPEG rejected: %+v %v", out, err)
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	truncated := data[:len(data)-30]
	if _, err := jpeg.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("truncation fixture must retain JPEG config: %v", err)
	}
	for _, invalid := range []string{
		"", "https://example.com/photo.jpg", "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes()),
		photoURL(pngData.Bytes()), photoURL([]byte("not a JPEG")), photoURL([]byte{0xff, 0xd8, 0xff}), photoURL(truncated),
		"data:image/jpeg;base64,", "data:image/jpeg;base64,!!!!", "data:image/jpeg;base64,Zh==",
		url + "\n", url + " ", strings.Replace(url, ";base64,", ";charset=utf-8;base64,", 1),
		strings.Replace(url, "image/jpeg", "image/jpg", 1), url[:len(url)-1],
	} {
		if _, err := ValidateCustomDesign(&CardCustomDesign{Photo: invalid}); !errors.Is(err, ErrCustomInvalid) {
			t.Fatalf("invalid photo accepted (%d bytes): %v", len(invalid), err)
		}
	}
	if _, err := ValidateCustomDesign(&CardCustomDesign{}); !errors.Is(err, ErrCustomInvalid) {
		t.Fatalf("empty custom accepted: %v", err)
	}
	if _, err := ValidateCustomDesign(&CardCustomDesign{DesignKey: "card-sage", Photo: url}); !errors.Is(err, ErrCustomInvalid) {
		t.Fatalf("preset and photo accepted together: %v", err)
	}
	if out, err := ValidateCustomDesign(nil); out != nil || err != nil {
		t.Fatalf("absent custom must allow regular designs: %+v %v", out, err)
	}
}

func TestCustomPhotoByteAndDimensionLimits(t *testing.T) {
	data := testJPEG(t, 8, 8)
	for _, size := range []int{customMaxImageBytes - 1, customMaxImageBytes, customMaxImageBytes + 1, customMaxImageBytes + 2, customMaxImageBytes + 3} {
		_, err := ValidateCustomDesign(&CardCustomDesign{Photo: photoURL(jpegWithSize(t, data, size))})
		if size <= customMaxImageBytes && err != nil {
			t.Fatalf("valid boundary JPEG rejected, bytes=%d: %v", size, err)
		}
		if size > customMaxImageBytes && !errors.Is(err, ErrCustomInvalid) {
			t.Fatalf("oversized JPEG accepted, bytes=%d: %v", size, err)
		}
	}
	for _, dims := range [][2]int{{customMaxDimension, 1}, {1, customMaxDimension}, {customMaxDimension + 1, 1}, {1, customMaxDimension + 1}} {
		_, err := ValidateCustomDesign(&CardCustomDesign{Photo: photoURL(testJPEG(t, dims[0], dims[1]))})
		if dims[0] <= customMaxDimension && dims[1] <= customMaxDimension && err != nil {
			t.Fatalf("dimension boundary rejected %v: %v", dims, err)
		}
		if (dims[0] > customMaxDimension || dims[1] > customMaxDimension) && !errors.Is(err, ErrCustomInvalid) {
			t.Fatalf("dimension limit ignored %v: %v", dims, err)
		}
	}
	bomb := append([]byte(nil), data...)
	sof := bytes.Index(bomb, []byte{0xff, 0xc0})
	if sof < 0 {
		t.Fatal("JPEG frame header missing")
	}
	binary.BigEndian.PutUint16(bomb[sof+5:sof+7], 65535)
	binary.BigEndian.PutUint16(bomb[sof+7:sof+9], 65535)
	if _, err := ValidateCustomDesign(&CardCustomDesign{Photo: photoURL(bomb)}); !errors.Is(err, ErrCustomInvalid) {
		t.Fatalf("JPEG dimension bomb accepted: %v", err)
	}
}

func TestDesignChoiceExclusivityAndOwnership(t *testing.T) {
	custom := &CardCustomDesign{DesignKey: "card-sage"}
	for _, choice := range []DesignChoice{
		{DesignID: -1}, {DesignID: BaseDesignCount},
		{DesignID: 1, DesignKey: "card-sage"}, {DesignID: 1, Custom: custom},
		{DesignIDSet: true, DesignKey: "card-sage"}, {DesignIDSet: true, Custom: custom},
		{DesignKey: "card-sage", Custom: custom},
	} {
		if _, err := ValidateDesignChoice(choice, nil, true); !errors.Is(err, ErrInvalidDesign) {
			t.Fatalf("conflicting/invalid choice accepted %+v: %v", choice, err)
		}
	}
	for id := 0; id < BaseDesignCount; id++ {
		if _, err := ValidateDesignChoice(DesignChoice{DesignID: id, DesignIDSet: true}, nil, false); err != nil {
			t.Fatalf("base design %d: %v", id, err)
		}
	}
	for _, design := range profile_sql.Catalog() {
		choice := DesignChoice{DesignKey: design.ID}
		if _, err := ValidateDesignChoice(choice, nil, false); !errors.Is(err, ErrDesignNotOwned) {
			t.Fatalf("regular unowned design %s: %v", design.ID, err)
		}
		_, err := ValidateDesignChoice(choice, map[string]bool{design.ID: true}, false)
		if design.AdminOnly && !errors.Is(err, ErrDesignNotOwned) {
			t.Fatalf("admin design role bypass %s: %v", design.ID, err)
		}
		if !design.AdminOnly && err != nil {
			t.Fatalf("regular owned design %s: %v", design.ID, err)
		}
		if _, err := ValidateDesignChoice(choice, nil, true); err != nil {
			t.Fatalf("existing admin behavior changed for %s: %v", design.ID, err)
		}
	}
	choice, err := ValidateDesignChoice(DesignChoice{DesignKey: " Card-Gold "}, map[string]bool{"card-gold": true}, false)
	if err != nil || choice.DesignKey != "card-gold" {
		t.Fatalf("regular key normalization failed: %+v %v", choice, err)
	}
	if _, err := ValidateDesignChoice(DesignChoice{DesignKey: "unknown"}, nil, true); !errors.Is(err, ErrInvalidDesign) {
		t.Fatalf("unknown regular design accepted: %v", err)
	}
}

func TestStoredCustomContractAndLegacyFallback(t *testing.T) {
	for _, raw := range []string{
		"", "null", "{}", "not json", `{"left":{"color":"#fff"},"right":{},"link":true}`,
		`{"nft":{"itemId":1,"rarity":"rare","art":{}}}`,
		`{"designKey":"card-sage","right":{}}`, `{"designKey":"card-gold"}`,
		`{"photo":"data:image/jpeg;base64,AAAA"}`, `{"designKey":"card-sage"} {}`,
	} {
		custom, err := scanCustom([]byte(raw))
		if err != nil || custom != nil {
			t.Fatalf("legacy/invalid stored JSON must fall back: %q %+v %v", raw, custom, err)
		}
	}
	for _, input := range []*CardCustomDesign{{DesignKey: "card-sage"}, {Photo: photoURL(testJPEG(t, 8, 8))}} {
		raw := customJSON(input).([]byte)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 1 {
			t.Fatalf("custom storage must contain one field: %s %v", raw, err)
		}
		custom, err := scanCustom(raw)
		if err != nil || custom == nil || *custom != *input {
			t.Fatalf("new custom roundtrip failed: %+v %v", custom, err)
		}
	}
	if customJSON(nil) != nil {
		t.Fatal("nil custom must remain SQL NULL")
	}
}

func TestChargeCustomRejectsInvalidBeforeDatabase(t *testing.T) {
	for _, custom := range []*CardCustomDesign{nil, {}, {DesignKey: "card-gold"}, {DesignKey: "card-dusk"}, {DesignKey: "card-xlavelia"}, {Photo: "invalid"}} {
		if _, err := ChargeCustomDesign(context.Background(), nil, 1, 0, custom); !errors.Is(err, ErrCustomInvalid) {
			t.Fatalf("invalid custom reached database: %+v %v", custom, err)
		}
	}
}

type chargeRow struct {
	voucherType string
	err         error
}

func (r chargeRow) Scan(dst ...any) error {
	if r.err == nil {
		*dst[0].(*string) = r.voucherType
	}
	return r.err
}

type chargeTx struct {
	pgx.Tx
	voucherType string
	voucherErr  error
	consumed    int
	charges     []float64
	affected    int64
}

func (tx *chargeTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if !strings.Contains(sql, "DELETE FROM user_vouchers") || len(args) != 2 || args[0] != 7 || args[1] != 42 {
		return chargeRow{err: errors.New("unexpected voucher query")}
	}
	tx.consumed++
	return chargeRow{voucherType: tx.voucherType, err: tx.voucherErr}
}

func (tx *chargeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !strings.Contains(sql, "lavx_balance >= $2") || len(args) != 2 || args[0] != 42 {
		return pgconn.CommandTag{}, errors.New("unexpected charge query")
	}
	tx.charges = append(tx.charges, args[1].(float64))
	if tx.affected == 1 {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func TestCustomPriceVouchersAndAdminBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, voucherType string
		admin             bool
		voucherID         int
		price             float64
	}{
		{"full-price", "", false, 0, 50}, {"discount-20", "custom_20", false, 7, 40},
		{"discount-50", "custom_50", false, 7, 25}, {"free-voucher", "custom_free", false, 7, 0},
		{"admin", "", true, 0, 0}, {"admin-voucher", "custom_20", true, 7, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &chargeTx{voucherType: tc.voucherType, affected: 1}
			if err := chargeCustomTx(context.Background(), tx, 42, tc.admin, tc.voucherID); err != nil {
				t.Fatal(err)
			}
			if tx.consumed != btoi(tc.voucherID > 0) || len(tx.charges) != btoi(tc.price > 0) {
				t.Fatalf("wrong voucher/charge count: %+v", tx)
			}
			if tc.price > 0 && tx.charges[0] != tc.price {
				t.Fatalf("price=%v want=%v", tx.charges[0], tc.price)
			}
		})
	}
	for _, admin := range []bool{false, true} {
		tx := &chargeTx{voucherErr: pgx.ErrNoRows, affected: 1}
		if err := chargeCustomTx(context.Background(), tx, 42, admin, 7); !errors.Is(err, ErrVoucherInvalid) || len(tx.charges) != 0 {
			t.Fatalf("invalid voucher behavior changed admin=%v: %v", admin, err)
		}
	}
	tx := &chargeTx{affected: 0}
	if err := chargeCustomTx(context.Background(), tx, 42, false, 0); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("insufficient balance accepted: %v", err)
	}
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}
