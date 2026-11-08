package profile_sql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type testRow struct {
	values []any
	err    error
}

func (r testRow) Scan(dst ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dst) != len(r.values) {
		return fmt.Errorf("scan: %d destinations, %d values", len(dst), len(r.values))
	}
	for i, value := range r.values {
		v := reflect.ValueOf(dst[i]).Elem()
		if value == nil {
			v.SetZero()
			continue
		}
		src := reflect.ValueOf(value)
		if v.Kind() == reflect.Pointer && src.Type() != v.Type() {
			v.Set(reflect.New(v.Type().Elem()))
			v = v.Elem()
		}
		if !src.Type().ConvertibleTo(v.Type()) {
			return fmt.Errorf("cannot scan %T into %s", value, v.Type())
		}
		v.Set(src.Convert(v.Type()))
	}
	return nil
}

type testRows struct {
	pgx.Rows
	data  [][]any
	index int
	err   error
}

func (r *testRows) Next() bool {
	if r.index >= len(r.data) {
		return false
	}
	r.index++
	return true
}
func (r *testRows) Scan(dst ...any) error { return (testRow{values: r.data[r.index-1]}).Scan(dst...) }
func (r *testRows) Close()                {}
func (r *testRows) Err() error            { return r.err }

type scriptedTx struct {
	pgx.Tx
	exec              func(string, ...any) (pgconn.CommandTag, error)
	query             func(string, ...any) (pgx.Rows, error)
	row               func(string, ...any) pgx.Row
	committed, rolled bool
}

func (tx *scriptedTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	return tx.exec(q, args...)
}
func (tx *scriptedTx) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	return tx.query(q, args...)
}
func (tx *scriptedTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	return tx.row(q, args...)
}
func (tx *scriptedTx) Commit(context.Context) error { tx.committed = true; return nil }
func (tx *scriptedTx) Rollback(context.Context) error {
	if !tx.committed {
		tx.rolled = true
	}
	return nil
}

type scriptedDB struct {
	database
	tx pgx.Tx
}

func (db scriptedDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) { return db.tx, nil }

func TestUpdateKeepsExistingNames(t *testing.T) {
	for _, tc := range []struct {
		name, current, next string
		taken               bool
		want                error
	}{
		{"unchanged-duplicate", "Player", "Player", true, nil},
		{"unchanged-long", strings.Repeat("界", 32), strings.Repeat("界", 32), true, nil},
		{"new-long", "Player", strings.Repeat("界", 25), false, ErrInvalid},
		{"new-taken", "Player", "Other", true, ErrUsernameTaken},
		{"new-valid", "Player", "Nova", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := tc.current
			renames, checks := 0, 0
			tx := &scriptedTx{}
			tx.row = func(q string, args ...any) pgx.Row {
				switch {
				case strings.Contains(q, "SELECT u.id,u.player_id,u.is_admin,"):
					return testRow{values: []any{1, "123456", false, false}}
				case q == "SELECT username FROM users WHERE id=$1":
					return testRow{values: []any{current}}
				case strings.Contains(q, "SELECT EXISTS(SELECT 1 FROM users WHERE id<>"):
					checks++
					return testRow{values: []any{tc.taken}}
				case strings.Contains(q, "SELECT u.id,u.player_id,u.username,"):
					return testRow{values: []any{1, "123456", current, "New bio", nil, DefaultTheme, DefaultEmblem, DefaultPattern, false, time.Unix(0, 0).UTC(), false, nil, nil, nil}}
				case strings.Contains(q, "SELECT lavx_balance::double precision FROM wallets"):
					return testRow{values: []any{100.0}}
				default:
					t.Fatalf("unexpected query %s", q)
					return testRow{err: ErrInvalid}
				}
			}
			tx.query = func(q string, args ...any) (pgx.Rows, error) {
				return &testRows{}, nil
			}
			tx.exec = func(q string, args ...any) (pgconn.CommandTag, error) {
				if strings.HasPrefix(q, "UPDATE users SET username=") {
					renames++
					current = args[1].(string)
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			store := &Store{db: scriptedDB{tx: tx}}
			result, err := store.Update(context.Background(), "123456", UpdateInput{Username: tc.next, Bio: "New bio", ShowBalance: false})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.next == tc.current && (renames != 0 || checks != 0) {
				t.Fatal("unchanged name was revalidated as a rename")
			}
			if tc.want == nil && (!tx.committed || result.Profile.Username != tc.next || result.Profile.ShowBalance) {
				t.Fatal("profile update was not preserved")
			}
			if tc.want != nil && !tx.rolled {
				t.Fatal("invalid rename did not roll back")
			}
		})
	}
}

func TestGrantRejectsAdminWithoutSQL(t *testing.T) {
	for _, id := range []string{"theme-xlavelia", "theme-wave", "emblem-sigil", "pattern-monogram"} {
		added, err := GrantCosmeticTx(context.Background(), nil, 1, id, "admin")
		if added || !errors.Is(err, ErrForbidden) {
			t.Fatalf("admin grant %s: %v %v", id, added, err)
		}
	}
	for _, args := range []struct {
		id, source string
		user       int
	}{{"unknown", "reward", 1}, {"theme-sage", "", 1}, {"theme-sage", "bad\nsource", 1}, {"theme-sage", "reward", 0}} {
		added, err := GrantCosmeticTx(context.Background(), nil, args.user, args.id, args.source)
		if added || !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid grant allowed: %+v", args)
		}
	}
}

func TestGrantConflictAndAccountBan(t *testing.T) {
	for _, tc := range []struct {
		name     string
		banned   bool
		affected int
		wantErr  error
		want     bool
	}{
		{"new", false, 1, nil, true}, {"duplicate", false, 0, nil, false}, {"banned", true, 0, ErrForbidden, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			tx := &scriptedTx{row: func(q string, args ...any) pgx.Row {
				if !strings.Contains(q, "banned_devices") {
					t.Fatal("did not reread ban status")
				}
				return testRow{values: []any{tc.banned}}
			}, exec: func(q string, args ...any) (pgconn.CommandTag, error) {
				writes++
				if q != grantCosmeticSQL || !strings.Contains(q, "ON CONFLICT (user_id, cosmetic_id) DO NOTHING") {
					t.Fatal(q)
				}
				if !reflect.DeepEqual(args, []any{1, "theme-sage", "battlepass:free:5"}) {
					t.Fatal(args)
				}
				return pgconn.NewCommandTag(fmt.Sprintf("INSERT 0 %d", tc.affected)), nil
			}}
			added, err := GrantCosmeticTx(context.Background(), tx, 1, "theme-sage", "battlepass:free:5")
			if added != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("%v %v", added, err)
			}
			if tc.banned && writes != 0 {
				t.Fatal("banned account got entitlement")
			}
			if tx.committed || tx.rolled {
				t.Fatal("helper controls caller transaction")
			}
		})
	}
}

func purchaseScript(t *testing.T, balance float64, owned, conflict bool, debitErr error) (*Store, *scriptedTx, *[]string) {
	t.Helper()
	calls := []string{}
	entitlement := owned
	tx := &scriptedTx{}
	tx.row = func(q string, args ...any) pgx.Row {
		calls = append(calls, q)
		switch {
		case strings.Contains(q, "SELECT u.id,u.player_id,u.is_admin,"):
			if !strings.Contains(q, "FOR NO KEY UPDATE OF u") {
				t.Fatal("mutation did not lock/reread user")
			}
			return testRow{values: []any{1, "123456", false, false}}
		case q == lockWalletSQL:
			return testRow{values: []any{balance}}
		case strings.HasPrefix(q, "SELECT "+bannedSQL):
			return testRow{values: []any{false}}
		case strings.Contains(q, "SELECT u.id,u.player_id,u.username,"):
			return testRow{values: []any{1, "123456", "Player", "", nil, DefaultTheme, DefaultEmblem, DefaultPattern, false, time.Unix(0, 0).UTC(), true, 42.0, nil, nil}}
		case strings.Contains(q, "SELECT lavx_balance::double precision FROM wallets"):
			return testRow{values: []any{balance}}
		default:
			t.Fatalf("unexpected query: %s", q)
			return testRow{err: errors.New("unexpected query")}
		}
	}
	tx.query = func(q string, args ...any) (pgx.Rows, error) {
		calls = append(calls, q)
		if q != ownershipSQL {
			t.Fatalf("unexpected rows query %s", q)
		}
		data := [][]any{}
		if entitlement {
			data = append(data, []any{"theme-silk"})
		}
		return &testRows{data: data}, nil
	}
	tx.exec = func(q string, args ...any) (pgconn.CommandTag, error) {
		calls = append(calls, q)
		switch q {
		case grantCosmeticSQL:
			if !reflect.DeepEqual(args, []any{1, "theme-silk", "shop"}) {
				t.Fatal(args)
			}
			entitlement = true
			if conflict {
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		case debitWalletSQL:
			if !reflect.DeepEqual(args, []any{1, 50}) {
				t.Fatal("untrusted price", args)
			}
			if debitErr != nil {
				return pgconn.CommandTag{}, debitErr
			}
			if balance < 50 {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			balance -= 50
			return pgconn.NewCommandTag("UPDATE 1"), nil
		default:
			t.Fatalf("unexpected exec %s", q)
			return pgconn.CommandTag{}, errors.New("unexpected exec")
		}
	}
	return &Store{db: scriptedDB{tx: tx}}, tx, &calls
}

func TestPurchaseAtomicity(t *testing.T) {
	for _, tc := range []struct {
		name              string
		balance           float64
		owned, conflict   bool
		debitErr, wantErr error
		wantDebit         bool
	}{
		{"new", 100, false, false, nil, nil, true},
		{"already-owned-no-funds", 0, true, false, nil, nil, false},
		{"racing-reward-grant", 100, false, true, nil, nil, false},
		{"insufficient", 49, false, false, nil, ErrInsufficientLavx, true},
		{"debit-failure", 100, false, false, errors.New("debit failed"), errors.New("debit failed"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, tx, calls := purchaseScript(t, tc.balance, tc.owned, tc.conflict, tc.debitErr)
			state, err := store.Purchase(context.Background(), "123456", "theme-silk")
			if tc.wantErr != nil {
				if err == nil || err.Error() != tc.wantErr.Error() || !tx.rolled || tx.committed {
					t.Fatalf("must rollback: err=%v rolled=%v committed=%v", err, tx.rolled, tx.committed)
				}
			} else if err != nil || !tx.committed || tx.rolled {
				t.Fatalf("must commit: %v", err)
			}
			debits, grantIndex, lockIndex, debitIndex := 0, -1, -1, -1
			for i, q := range *calls {
				if q == lockWalletSQL {
					lockIndex = i
				}
				if q == grantCosmeticSQL {
					grantIndex = i
				}
				if q == debitWalletSQL {
					debits++
					debitIndex = i
				}
			}
			if (debits == 1) != tc.wantDebit || debits > 1 {
				t.Fatalf("debit count %d", debits)
			}
			if debits == 1 && !(lockIndex < grantIndex && grantIndex < debitIndex) {
				t.Fatal("lock/grant/debit order violated")
			}
			if err == nil {
				want := tc.balance
				if tc.wantDebit {
					want -= 50
				}
				if state.LavxBalance != want {
					t.Fatalf("balance %v want %v", state.LavxBalance, want)
				}
				silkOwned := false
				for _, c := range state.Catalog {
					if c.ID == "theme-silk" {
						silkOwned = c.Owned
					}
				}
				if !silkOwned {
					t.Fatal("purchase response omitted entitlement")
				}
			}
		})
	}
	store := &Store{}
	for _, id := range []string{"theme-xlavelia", "theme-wave", "theme-sage", DefaultTheme, "emblem-dragon"} {
		if _, err := store.Purchase(context.Background(), "123456", id); !errors.Is(err, ErrNotPurchasable) {
			t.Fatal(id, err)
		}
	}
}

func TestPrivateAccountValidity(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  testRow
		want error
	}{
		{"deleted", testRow{err: pgx.ErrNoRows}, ErrUnauthorized},
		{"banned", testRow{values: []any{1, "123456", false, true}}, ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &scriptedTx{row: func(q string, args ...any) pgx.Row { return tc.row }}
			store := &Store{db: scriptedDB{tx: tx}}
			if _, err := store.Own(context.Background(), "123456"); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if !tx.rolled || tx.committed {
				t.Fatal("invalid account committed")
			}
		})
	}
}

func TestPublicScannerRedactsAndUsesInlineJPEG(t *testing.T) {
	row := testRow{values: []any{2, "654321", "Player", "", []byte{0xff, 0xd8, 0xff}, DefaultTheme, DefaultEmblem, DefaultPattern, false, time.Unix(0, 0).UTC(), false, 123.0, 2, "pending"}}
	p, err := scanPublic(row, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Balance != nil || p.FriendState != "incoming" || !strings.HasPrefix(p.AvatarURL, "data:image/jpeg;base64,") {
		t.Fatalf("unsafe profile %+v", p)
	}
}

// Модель не проверяет блокировки PostgreSQL; их порядок проверяется отдельно.
type memoryPurchaseDB struct {
	database
	mu      sync.Mutex
	balance float64
	owned   bool
	debits  int
}
type memoryPurchaseTx struct {
	pgx.Tx
	db      *memoryPurchaseDB
	balance float64
	owned   bool
	debits  int
	done    bool
}

func (db *memoryPurchaseDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	db.mu.Lock()
	return &memoryPurchaseTx{db: db, balance: db.balance, owned: db.owned}, nil
}
func (tx *memoryPurchaseTx) Rollback(context.Context) error {
	if !tx.done {
		tx.done = true
		tx.db.mu.Unlock()
	}
	return nil
}
func (tx *memoryPurchaseTx) Commit(context.Context) error {
	tx.db.balance, tx.db.owned = tx.balance, tx.owned
	tx.db.debits += tx.debits
	tx.done = true
	tx.db.mu.Unlock()
	return nil
}
func (tx *memoryPurchaseTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "SELECT u.id,u.player_id,u.is_admin,"):
		return testRow{values: []any{1, "123456", false, false}}
	case strings.HasPrefix(q, "SELECT "+bannedSQL):
		return testRow{values: []any{false}}
	case q == lockWalletSQL || strings.Contains(q, "SELECT lavx_balance::double precision FROM wallets"):
		return testRow{values: []any{tx.balance}}
	case strings.Contains(q, "SELECT u.id,u.player_id,u.username,"):
		return testRow{values: []any{1, "123456", "Player", "", nil, DefaultTheme, DefaultEmblem, DefaultPattern, false, time.Unix(0, 0).UTC(), true, 0.0, nil, nil}}
	}
	return testRow{err: fmt.Errorf("unexpected query %s", q)}
}
func (tx *memoryPurchaseTx) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	if q != ownershipSQL {
		return nil, fmt.Errorf("unexpected rows query %s", q)
	}
	data := [][]any{}
	if tx.owned {
		data = append(data, []any{"theme-silk"})
	}
	return &testRows{data: data}, nil
}
func (tx *memoryPurchaseTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	if q == grantCosmeticSQL {
		if tx.owned {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		}
		tx.owned = true
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}
	if q == debitWalletSQL {
		if tx.balance < 50 {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		tx.balance -= 50
		tx.debits++
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.CommandTag{}, fmt.Errorf("unexpected exec %s", q)
}

func TestConcurrentPurchaseRetriesAndRollback(t *testing.T) {
	for _, balance := range []float64{100, 49} {
		db := &memoryPurchaseDB{balance: balance}
		store := &Store{db: db}
		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.Purchase(context.Background(), "123456", "theme-silk")
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if balance == 100 && err != nil {
				t.Fatal(err)
			}
			if balance == 49 && !errors.Is(err, ErrInsufficientLavx) {
				t.Fatal(err)
			}
		}
		if balance == 100 && (db.balance != 50 || !db.owned || db.debits != 1) {
			t.Fatalf("duplicate charge: %+v", db)
		}
		if balance == 49 && (db.balance != 49 || db.owned || db.debits != 0) {
			t.Fatalf("entitlement survived rollback: %+v", db)
		}
	}
}
