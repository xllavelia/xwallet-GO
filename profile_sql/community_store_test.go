package profile_sql

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFriendStoreIdentityAndTransitions(t *testing.T) {
	for _, tc := range []struct {
		name, viewer, target, current, action, want string
		wantErr                                     error
		requester                                   int
	}{
		{"new-request", "123456", "654321", "none", "request", "outgoing", nil, 0},
		{"cross-request", "654321", "123456", "pending", "request", "friend", nil, 1},
		{"receiver-accept", "654321", "123456", "pending", "accept", "friend", nil, 1},
		{"requester-cannot-accept", "123456", "654321", "pending", "accept", "", ErrTransition, 1},
		{"receiver-decline", "654321", "123456", "pending", "decline", "none", nil, 1},
		{"requester-cancel", "123456", "654321", "pending", "cancel", "none", nil, 1},
		{"receiver-cannot-cancel", "654321", "123456", "pending", "cancel", "", ErrTransition, 1},
		{"requester-cannot-decline", "123456", "654321", "pending", "decline", "", ErrTransition, 1},
		{"friend-remove", "123456", "654321", "friend", "remove", "none", nil, 1},
		{"request-retry", "123456", "654321", "pending", "request", "outgoing", nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			tx := &scriptedTx{}
			tx.query = func(q string, args ...any) (pgx.Rows, error) {
				if !strings.Contains(q, "ORDER BY u.id FOR NO KEY UPDATE OF u") || !strings.Contains(q, "banned_devices") {
					t.Fatal("accounts not validated/locked in stable order")
				}
				return &testRows{data: [][]any{{1, "123456", false, false}, {2, "654321", false, false}}}, nil
			}
			tx.row = func(q string, args ...any) pgx.Row {
				if !reflect.DeepEqual(args, []any{1, 2}) || !strings.Contains(q, "FOR UPDATE") {
					t.Fatal("pair not normalized/locked")
				}
				if tc.current == "none" {
					return testRow{err: pgx.ErrNoRows}
				}
				return testRow{values: []any{tc.requester, tc.current}}
			}
			tx.exec = func(q string, args ...any) (pgconn.CommandTag, error) {
				writes++
				if args[0] != 1 || args[1] != 2 {
					t.Fatal("pair changed", args)
				}
				switch tc.want {
				case "outgoing":
					if !strings.Contains(q, "INSERT INTO community_friendships") || len(args) != 3 || args[2] != 1 {
						t.Fatal("requester identity not retained", q, args)
					}
				case "friend":
					if !strings.Contains(q, "SET state='friend'") || strings.Contains(q, "requester_id=") {
						t.Fatal("accept overwrote requester identity", q)
					}
				case "none":
					if !strings.HasPrefix(q, "DELETE FROM community_friendships") {
						t.Fatal(q)
					}
				default:
					t.Fatal("forbidden transition wrote DB")
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			store := &Store{db: scriptedDB{tx: tx}}
			got, err := store.Friend(context.Background(), tc.viewer, tc.target, tc.action)
			if !errors.Is(err, tc.wantErr) || got.FriendState != tc.want {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if tc.wantErr != nil && (!tx.rolled || tx.committed || writes != 0) {
				t.Fatal("forbidden action did not rollback")
			}
			if tc.wantErr == nil && !tx.committed {
				t.Fatal("relationship not committed")
			}
			if tc.name == "request-retry" && writes != 0 {
				t.Fatal("retry wrote duplicate request")
			}
		})
	}
	if _, err := (&Store{}).Friend(context.Background(), "123456", "123456", "request"); !errors.Is(err, ErrTransition) {
		t.Fatal("self friendship allowed")
	}
}
