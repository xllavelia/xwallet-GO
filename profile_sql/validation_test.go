package profile_sql

import (
	"strings"
	"testing"
)

func TestProfileValidation(t *testing.T) {
	for _, name := range []string{"abc", strings.Repeat("界", 32), "Player Name"} {
		if ValidateUpdate(UpdateInput{Username: name, Bio: strings.Repeat("界", 160)}) != nil {
			t.Fatalf("valid name %q", name)
		}
	}
	for _, name := range []string{"ab", strings.Repeat("界", 33), " abc", "abc ", "a\nbc", "ab\x00c", "ab\u200bc", "ab\u202ec", string([]byte{0xff, 'a', 'b'})} {
		if ValidateUpdate(UpdateInput{Username: name}) == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	for _, bio := range []string{strings.Repeat("界", 161), "x\ny", "x\ty", "x\u007fy", "x\u0085y", "x\u200dy"} {
		if ValidateUpdate(UpdateInput{Username: "abc", Bio: bio}) == nil {
			t.Fatalf("accepted bio %q", bio)
		}
	}
	for _, id := range []string{"000001", "123456"} {
		if ValidatePlayerID(id) != nil {
			t.Fatal(id)
		}
	}
	for _, id := range []string{"12345", "1234567", "１２３４５６", "abc123", "12345 ", ""} {
		if ValidatePlayerID(id) == nil {
			t.Fatal(id)
		}
	}
	for _, q := range []string{"ab", strings.Repeat("界", 32), "%_"} {
		if ValidateSearch(q) != nil {
			t.Fatal(q)
		}
	}
	for _, q := range []string{"a", strings.Repeat("界", 33), " ab", "a\nb"} {
		if ValidateSearch(q) == nil {
			t.Fatal(q)
		}
	}
	if escapeSearch(`a%_\b`) != `a\%\_\\b` {
		t.Fatal("LIKE wildcard not escaped")
	}
}

func TestNewsPlainText(t *testing.T) {
	n := News{ID: "notice", Title: "Welcome", Summary: "Plain text", Body: "First line\nSecond line", Category: "Community", Art: "community"}
	if !ValidNews(n) {
		t.Fatal("valid notice rejected")
	}
	for _, body := range []string{"<script>alert(1)</script>", "Visit https://example.com", "www.example.com", "example.org", "example.xyz", "//example.invalid", "http:example", "javascript:alert(1)", "[click](javascript:alert(1))", "mailto:x@y", "bad\x00text", strings.Repeat("a", 8001)} {
		bad := n
		bad.Body = body
		if ValidNews(bad) {
			t.Fatalf("unsafe notice allowed: %q", body)
		}
	}
	for _, art := range []string{"custom", "https://example.com", ""} {
		bad := n
		bad.Art = art
		if ValidNews(bad) {
			t.Fatal(art)
		}
	}
}

func TestFriendTransitions(t *testing.T) {
	allowed := map[string]map[string]string{
		"none":     {"request": "outgoing"},
		"outgoing": {"request": "outgoing", "cancel": "none"},
		"incoming": {"request": "friend", "accept": "friend", "decline": "none"},
		"friend":   {"request": "friend", "accept": "friend", "remove": "none"},
	}
	for _, current := range []string{"none", "incoming", "outgoing", "friend", "self"} {
		for _, action := range []string{"request", "accept", "decline", "cancel", "remove", "message"} {
			want, ok := allowed[current][action]
			got, err := nextFriendState(current, action)
			if ok && (err != nil || got != want) {
				t.Fatalf("%s %s => %s %v", current, action, got, err)
			}
			if !ok && err == nil {
				t.Fatalf("unauthorized transition %s %s", current, action)
			}
		}
	}
	a, b := 1, 2
	pending, friend := "pending", "friend"
	if relationshipState(a, b, &a, &pending) != "outgoing" || relationshipState(b, a, &a, &pending) != "incoming" {
		t.Fatal("request identity reversed")
	}
	if relationshipState(a, b, &a, &friend) != "friend" || relationshipState(b, a, &a, &friend) != "friend" {
		t.Fatal("friendship not mutual")
	}
	if relationshipState(a, a, nil, nil) != "self" || relationshipState(a, b, nil, nil) != "none" {
		t.Fatal("relationship defaults")
	}
}

func TestSQLGuards(t *testing.T) {
	for _, fragment := range []string{"NOT u.is_admin", "w.balance DESC,u.player_id ASC", "COALESCE(p.show_balance,true)", "banned_devices", "u.banned"} {
		if !strings.Contains(rankingCTE, fragment) {
			t.Fatalf("rank missing %s", fragment)
		}
	}
	for _, fragment := range []string{"is_published", "published_at<=now()", "LIMIT 30"} {
		if !strings.Contains(newsListSQL, fragment) {
			t.Fatal(fragment)
		}
	}
	for _, fragment := range []string{"PRIMARY KEY (user_low, user_high)", "CHECK (user_low < user_high)", "CHECK (requester_id IN (user_low, user_high))", "ON DELETE CASCADE", "octet_length(avatar) <= 98304", "octet_length(avatar_thumb) <= 12288"} {
		if !strings.Contains(schemaSQL, fragment) {
			t.Fatal(fragment)
		}
	}
	if !strings.Contains(ownershipSQL, "FROM user_cosmetics") || strings.Contains(ownershipSQL, "user_statuses") {
		t.Fatal("ownership source changed")
	}
	if !strings.Contains(seedNewsSQL, "ON CONFLICT (id) DO UPDATE") {
		t.Fatal("seed no longer refreshes")
	}
	if !strings.Contains(publicColumns(false), "CASE WHEN COALESCE(p.show_balance,true) THEN w.balance") {
		t.Fatal("privacy absent from SQL")
	}
	if !strings.Contains(publicColumns(true), "p.avatar_thumb") || strings.Contains(publicColumns(true), "p.avatar,") {
		t.Fatal("list uses full avatars")
	}
}
