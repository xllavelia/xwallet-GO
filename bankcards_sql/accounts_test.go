package bankcards_sql

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidateOpenCard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		design int
		want   string
		err    error
	}{
		{"  Savings  ", 5, "Savings", nil},
		{"\u2003旅行\u2003", 9, "旅行", nil},
		{strings.Repeat("界", 32), 7, strings.Repeat("界", 32), nil},
		{"", 5, "", ErrInvalidName},
		{"   ", 5, "", ErrInvalidName},
		{strings.Repeat("界", 33), 5, "", ErrInvalidName},
		{"a\x00b", 5, "", ErrInvalidName},
		{"\tName", 5, "", ErrInvalidName},
		{"Name\n", 5, "", ErrInvalidName},
		{"a\u0085b", 5, "", ErrInvalidName},
		{string([]byte{0xff}), 5, "", ErrInvalidName},
		{"Name", 4, "", ErrInvalidDesign},
		{"Name", 10, "", ErrInvalidDesign},
	} {
		got, err := ValidateOpenCard(tc.name, tc.design)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("ValidateOpenCard(%q, %d) = %q, %v; want %q, %v", tc.name, tc.design, got, err, tc.want, tc.err)
		}
	}
	for design := 5; design <= 9; design++ {
		if _, err := ValidateOpenCard("Account", design); err != nil {
			t.Errorf("design %d: %v", design, err)
		}
	}
}

func TestValidateCardAmount(t *testing.T) {
	for _, amount := range []float64{0.01, 0.29, 1.23, 12.34, 100, 999999999999.99} {
		if err := ValidateCardAmount(amount); err != nil {
			t.Errorf("valid amount %v: %v", amount, err)
		}
	}
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 0.001, 0.005, 1.234, 1e12} {
		if err := ValidateCardAmount(amount); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("invalid amount %v: %v", amount, err)
		}
		if err := TopUpCard(context.Background(), nil, 1, 1, amount); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("topup amount %v: %v", amount, err)
		}
		if err := WithdrawFromCard(context.Background(), nil, 1, 1, amount); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("withdraw amount %v: %v", amount, err)
		}
	}
	if _, err := OpenCard(context.Background(), nil, 1, "", 5); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("open validation: %v", err)
	}
}

type sqlStep struct {
	contains string
	args     []interface{}
	values   []interface{}
	err      error
	tag      string
	check    func(string, []interface{})
}

type scriptedTx struct {
	pgx.Tx // Неожиданный вызов неподдерживаемого метода должен завершить тест.
	t      *testing.T
	steps  []sqlStep
}

func newScriptedTx(t *testing.T, steps ...sqlStep) *scriptedTx {
	t.Helper()
	tx := &scriptedTx{t: t, steps: steps}
	t.Cleanup(func() {
		if len(tx.steps) != 0 {
			t.Errorf("%d SQL operations were not executed", len(tx.steps))
		}
	})
	return tx
}

func (tx *scriptedTx) next(sql string, args []interface{}) sqlStep {
	tx.t.Helper()
	if len(tx.steps) == 0 {
		tx.t.Fatalf("unexpected SQL: %s", sql)
	}
	step := tx.steps[0]
	tx.steps = tx.steps[1:]
	if !strings.Contains(sql, step.contains) {
		tx.t.Fatalf("SQL %q does not contain %q", sql, step.contains)
	}
	if step.args != nil && !reflect.DeepEqual(args, step.args) {
		tx.t.Fatalf("SQL args %v; want %v", args, step.args)
	}
	if step.check != nil {
		step.check(sql, args)
	}
	return step
}

func (tx *scriptedTx) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	return scriptedRow{step: tx.next(sql, args)}
}

func (tx *scriptedTx) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	step := tx.next(sql, args)
	return pgconn.NewCommandTag(step.tag), step.err
}

type scriptedRow struct{ step sqlStep }

func (r scriptedRow) Scan(dest ...interface{}) error {
	if r.step.err != nil {
		return r.step.err
	}
	if len(dest) != len(r.step.values) {
		return errors.New("unexpected scan arity")
	}
	for i, value := range r.step.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

func userLock() sqlStep {
	return sqlStep{contains: "SELECT id FROM users WHERE id = $1 FOR UPDATE", args: []interface{}{7}, values: []interface{}{7}}
}

func cardLock() sqlStep {
	return sqlStep{contains: "SELECT id FROM bank_cards WHERE id = $1 AND user_id = $2 FOR UPDATE", args: []interface{}{42, 7}, values: []interface{}{42}}
}

func TestOpenCardLockedLimits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		used  bool
		want  error
	}{
		{"sixth", 5, false, ErrMaxCardsReached},
		{"legacy overlimit", 8, true, ErrMaxCardsReached},
		{"duplicate design", 2, true, ErrDesignAlreadyOwned},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newScriptedTx(t, userLock(), sqlStep{
				contains: "COUNT(*), COALESCE(BOOL_OR(design_id = $2), false)",
				args:     []interface{}{7, 5}, values: []interface{}{tc.count, tc.used},
			})
			if _, err := openCardTx(context.Background(), tx, 7, "Savings", 5); !errors.Is(err, tc.want) {
				t.Fatalf("open: %v; want %v", err, tc.want)
			}
		})
	}
}

func TestOpenCardFreeInactiveAndCollisionRetry(t *testing.T) {
	opened := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	insertCheck := func(sql string, args []interface{}) {
		if !strings.Contains(sql, "VALUES ($1, 'standard', $2, $3, $4, 0, false)") {
			t.Fatal("new account must be standard, zero balance, inactive")
		}
		if !strings.Contains(sql, "ON CONFLICT (card_number) DO NOTHING") {
			t.Fatal("collision must not abort transaction")
		}
		if len(args) != 4 || args[0] != 7 || args[1] != "Savings" || args[2] != 5 {
			t.Fatalf("insert args: %v", args)
		}
		number := args[3].(string)
		if len(number) != 16 || strings.Trim(number, "0123456789") != "" {
			t.Fatalf("invalid number %q", number)
		}
	}
	tx := newScriptedTx(t, userLock(),
		sqlStep{contains: "COUNT(*)", values: []interface{}{4, false}},
		sqlStep{contains: "INSERT INTO bank_cards", err: pgx.ErrNoRows, check: insertCheck},
		sqlStep{contains: "INSERT INTO bank_cards", check: insertCheck, values: []interface{}{42, 7, "standard", "Savings", 5, "1234567890123456", 0.0, false, opened}},
	)
	card, err := openCardTx(context.Background(), tx, 7, "Savings", 5)
	if err != nil {
		t.Fatal(err)
	}
	if card.ID != 42 || card.Name != "Savings" || card.DesignID != 5 || card.Balance != 0 || card.IsActiveForTrading || card.OpenedAt != opened {
		t.Fatalf("unexpected card: %+v", card)
	}
}

func TestSelectWalletAndOwnedCard(t *testing.T) {
	t.Run("wallet", func(t *testing.T) {
		tx := newScriptedTx(t, userLock(), sqlStep{contains: "SET is_active_for_trading = false WHERE user_id = $1", args: []interface{}{7}, tag: "UPDATE 5"})
		if err := selectActiveCardTx(context.Background(), tx, 7, 0); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("card", func(t *testing.T) {
		tx := newScriptedTx(t, userLock(), cardLock(),
			sqlStep{contains: "SET is_active_for_trading = false", tag: "UPDATE 5"},
			sqlStep{contains: "SET is_active_for_trading = true WHERE id = $1", args: []interface{}{42}, tag: "UPDATE 1"},
		)
		if err := selectActiveCardTx(context.Background(), tx, 7, 42); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("not owned", func(t *testing.T) {
		step := cardLock()
		step.err = pgx.ErrNoRows
		tx := newScriptedTx(t, userLock(), step)
		if err := selectActiveCardTx(context.Background(), tx, 7, 42); !errors.Is(err, ErrCardNotFound) {
			t.Fatal(err)
		}
	})
}

func TestCloseCardLockedGuards(t *testing.T) {
	for _, balance := range []float64{0.01, 12.34, -1} {
		t.Run("funded", func(t *testing.T) {
			tx := newScriptedTx(t, userLock(), sqlStep{contains: "SELECT balance FROM bank_cards WHERE id = $1 AND user_id = $2 FOR UPDATE", args: []interface{}{42, 7}, values: []interface{}{balance}})
			if err := closeCardTx(context.Background(), tx, 7, 42); !errors.Is(err, ErrCardFunded) {
				t.Fatal(err)
			}
		})
	}
	for _, hasOpen := range []bool{true, false} {
		t.Run("positions", func(t *testing.T) {
			steps := []sqlStep{userLock(),
				{contains: "SELECT balance FROM bank_cards", values: []interface{}{0.0}},
				{contains: "funding_card_id = $1 AND status = 'open'", args: []interface{}{42}, values: []interface{}{hasOpen}},
			}
			if !hasOpen {
				steps = append(steps, sqlStep{contains: "DELETE FROM bank_cards WHERE id = $1 AND user_id = $2", args: []interface{}{42, 7}, tag: "DELETE 1"})
			}
			tx := newScriptedTx(t, steps...)
			err := closeCardTx(context.Background(), tx, 7, 42)
			if hasOpen && !errors.Is(err, ErrCardHasOpenPositions) || !hasOpen && err != nil {
				t.Fatalf("close: %v", err)
			}
		})
	}
	t.Run("not owned", func(t *testing.T) {
		tx := newScriptedTx(t, userLock(), sqlStep{contains: "AND user_id = $2 FOR UPDATE", err: pgx.ErrNoRows})
		if err := closeCardTx(context.Background(), tx, 7, 42); !errors.Is(err, ErrCardNotFound) {
			t.Fatal(err)
		}
	})
}

func TestFundingActiveOrWalletFallback(t *testing.T) {
	for _, active := range []bool{true, false} {
		step := sqlStep{contains: "is_active_for_trading = true", args: []interface{}{7}, values: []interface{}{42}}
		want := FundingSource{Kind: "card", CardID: 42, UserID: 7}
		if !active {
			step.err = pgx.ErrNoRows
			want = FundingSource{Kind: "wallet", UserID: 7}
		}
		tx := newScriptedTx(t, step)
		source, err := ResolveAnyCard(context.Background(), tx, 7)
		if err != nil || source != want {
			t.Fatalf("source = %+v, %v; want %+v", source, err, want)
		}
	}
	dbErr := errors.New("database unavailable")
	tx := newScriptedTx(t, sqlStep{contains: "is_active_for_trading = true", err: dbErr})
	if _, err := ResolveAnyCard(context.Background(), tx, 7); !errors.Is(err, dbErr) {
		t.Fatal(err)
	}
}

func TestFundingCreditsRequireOneRow(t *testing.T) {
	for _, source := range []FundingSource{{Kind: "card", CardID: 42}, {Kind: "wallet", UserID: 7}} {
		for _, tag := range []string{"UPDATE 0", "UPDATE 1"} {
			tx := newScriptedTx(t, sqlStep{contains: "SET balance = balance + $1", tag: tag})
			err := AdjustFundingBalance(context.Background(), tx, source, 12.34)
			if tag == "UPDATE 0" && !errors.Is(err, ErrFundingSourceNotFound) || tag == "UPDATE 1" && err != nil {
				t.Fatalf("credit %s: %v", tag, err)
			}
		}
	}
	for _, delta := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := AdjustFundingBalance(context.Background(), nil, FundingSource{}, delta); !errors.Is(err, ErrInvalidFundingDelta) {
			t.Fatal(err)
		}
	}
	tx := newScriptedTx(t, sqlStep{contains: "SET balance = balance + $1", err: errors.New("credit failed")})
	if err := AdjustFundingBalance(context.Background(), tx, FundingSource{Kind: "card", CardID: 42}, 1); err == nil {
		t.Fatal("credit error swallowed")
	}
}

func TestCardMoneyMovement(t *testing.T) {
	for _, topUp := range []bool{true, false} {
		fromTable, toTable := "wallets", "bank_cards"
		fromID, toID := 7, 42
		if !topUp {
			fromTable, toTable = toTable, fromTable
			fromID, toID = toID, fromID
		}
		for _, failure := range []string{"", "insufficient", "missing credit"} {
			steps := []sqlStep{userLock(), cardLock(), {contains: "UPDATE " + fromTable, args: []interface{}{-12.34, fromID, 12.34}, values: []interface{}{87.66}}}
			want := error(nil)
			if failure == "insufficient" {
				steps[2].err = pgx.ErrNoRows
				want = ErrInsufficientFunds
			} else {
				tag := "UPDATE 1"
				if failure == "missing credit" {
					tag = "UPDATE 0"
					want = ErrFundingSourceNotFound
				}
				steps = append(steps, sqlStep{contains: "UPDATE " + toTable, args: []interface{}{12.34, toID}, tag: tag})
			}
			tx := newScriptedTx(t, steps...)
			if err := moveCardFundsTx(context.Background(), tx, 7, 42, 12.34, topUp); !errors.Is(err, want) {
				t.Fatalf("topup=%v failure=%q: %v; want %v", topUp, failure, err, want)
			}
		}
	}
}
