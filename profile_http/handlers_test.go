package profile_http

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xwallet-server/profile_sql"

	"github.com/golang-jwt/jwt/v5"
)

type stubService struct {
	calls         int
	player        string
	update        profile_sql.UpdateInput
	avatar, thumb []byte
	err           error
}

func (s *stubService) touch(player string) { s.calls++; s.player = player }
func (s *stubService) state() profile_sql.OwnState {
	return profile_sql.OwnState{Profile: profile_sql.PublicProfile{PlayerID: "123456", Username: "LiveName", Bio: "", ThemeID: profile_sql.DefaultTheme,
		EmblemID: profile_sql.DefaultEmblem, PatternID: profile_sql.DefaultPattern, ShowBalance: false, FriendState: "self", IsAdmin: false},
		Catalog: profile_sql.CatalogFor(nil, false), LavxBalance: 100}
}
func (s *stubService) Own(_ context.Context, p string) (profile_sql.OwnState, error) {
	s.touch(p)
	return s.state(), s.err
}
func (s *stubService) Update(_ context.Context, p string, in profile_sql.UpdateInput) (profile_sql.OwnState, error) {
	s.touch(p)
	s.update = in
	return s.state(), s.err
}
func (s *stubService) Equip(_ context.Context, p string, in profile_sql.EquipInput) (profile_sql.OwnState, error) {
	s.touch(p)
	return s.state(), s.err
}
func (s *stubService) Purchase(_ context.Context, p, id string) (profile_sql.OwnState, error) {
	s.touch(p)
	return s.state(), s.err
}
func (s *stubService) View(_ context.Context, p, target string) (profile_sql.PublicProfile, error) {
	s.touch(p)
	state := s.state().Profile
	state.PlayerID = target
	state.FriendState = "none"
	return state, s.err
}
func (s *stubService) SetAvatar(_ context.Context, p string, a, thumb []byte) (profile_sql.OwnState, error) {
	s.touch(p)
	s.avatar, s.thumb = a, thumb
	return s.state(), s.err
}
func (s *stubService) Community(_ context.Context, p string) (profile_sql.CommunityState, error) {
	s.touch(p)
	return profile_sql.CommunityState{
		News: []profile_sql.News{}, Friends: []profile_sql.PublicProfile{}, Incoming: []profile_sql.PublicProfile{}, Outgoing: []profile_sql.PublicProfile{}, Leaderboard: []profile_sql.LeaderboardEntry{}}, s.err
}
func (s *stubService) Search(_ context.Context, p, q string) ([]profile_sql.PublicProfile, error) {
	s.touch(p)
	return []profile_sql.PublicProfile{}, s.err
}
func (s *stubService) Friend(_ context.Context, p, target, action string) (profile_sql.Relationship, error) {
	s.touch(p)
	return profile_sql.Relationship{FriendState: "outgoing"}, s.err
}

func testMux(t *testing.T, s service) (*http.ServeMux, string) {
	t.Helper()
	t.Setenv("JWT_SECRET", "profile-http-unit-test-secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"playerId": "123456", "username": "StaleName", "isAdmin": true, "exp": time.Now().Add(time.Hour).Unix()})
	signed, err := token.SignedString([]byte("profile-http-unit-test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	register(mux, s)
	return mux, signed
}
func perform(mux *http.ServeMux, token, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestAllRoutesRequireAuthAndMethods(t *testing.T) {
	s := &stubService{}
	mux, token := testMux(t, s)
	for path, method := range map[string]string{"/profile": "GET", "/profile/update": "POST", "/profile/equip": "POST", "/profile/purchase": "POST", "/profile/view?playerId=654321": "GET", "/profile/avatar": "POST", "/community": "GET", "/community/search?q=ab": "GET", "/community/friend": "POST"} {
		if w := perform(mux, "", method, path, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %d", path, w.Code)
		}
		if w := perform(mux, token, "PUT", path, ""); w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("wrong method %s %d", path, w.Code)
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid requests reached store")
	}
}

func TestHTTPValidationAndIDImmutability(t *testing.T) {
	s := &stubService{}
	mux, token := testMux(t, s)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/profile/update", `{"username":"Player","bio":"","showBalance":true,"playerId":"654321"}`, 400},
		{"/profile/update", `{"username":"ab","bio":"","showBalance":true}`, 400},
		{"/profile/update", `{"username":"Player","bio":"bad\ntext","showBalance":true}`, 400},
		{"/profile/update", `{"username":"Player","bio":""}`, 400},
		{"/profile/update", `{"username":null,"bio":"","showBalance":true}`, 400},
		{"/profile/update", `{"username":"Player","bio":"","showBalance":true} {}`, 400},
		{"/profile/update", "null", 400},
		{"/profile/update", string([]byte{'{', 0xff, '}'}), 400},
		{"/profile/equip", `{"themeId":"tide","emblemId":"emblem-none","patternId":"pattern-orbit"}`, 400},
		{"/profile/equip", `{"themeId":"emblem-fox","emblemId":"emblem-none","patternId":"pattern-orbit"}`, 400},
		{"/profile/purchase", `{"cosmeticId":"theme-xlavelia"}`, 409},
		{"/profile/purchase", `{"cosmeticId":"theme-sage"}`, 409},
		{"/profile/purchase", `{"cosmeticId":"theme-silk","price":0}`, 400},
		{"/profile/purchase", `{"cosmeticId":"unknown"}`, 400},
		{"/profile/avatar", `{"image":"https://example.com/a.jpg"}`, 400},
		{"/profile/avatar", strings.Repeat(" ", AvatarMaxRequestBytes+1), 413},
		{"/profile/view?playerId=not-valid", "", 400},
		{"/community/search?q=a", "", 400},
		{"/community/friend", `{"playerId":"654321","action":"message"}`, 400},
		{"/community/friend", `{"playerId":"65432","action":"request"}`, 400},
	} {
		method := "POST"
		if strings.Contains(tc.path, "?") {
			method = "GET"
		}
		w := perform(mux, token, method, tc.path, tc.body)
		if w.Code != tc.status {
			t.Fatalf("%s body=%s: %d %s", tc.path, tc.body[:min(len(tc.body), 100)], w.Code, w.Body.String())
		}
	}
	if s.calls != 0 {
		t.Fatal("validation failed before private operation")
	}
}

func TestHTTPResponseShapesAndJWTNotAuthoritative(t *testing.T) {
	s := &stubService{}
	mux, token := testMux(t, s)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/profile", ""}, {"POST", "/profile/update", `{"username":"NewName","bio":"","showBalance":false}`},
		{"POST", "/profile/equip", `{"themeId":"theme-tide","emblemId":"emblem-none","patternId":"pattern-orbit"}`},
		{"POST", "/profile/purchase", `{"cosmeticId":"theme-silk"}`}, {"DELETE", "/profile/avatar", ""},
	} {
		w := perform(mux, token, tc.method, tc.path, tc.body)
		if w.Code != 200 {
			t.Fatalf("%s %d %s", tc.path, w.Code, w.Body.String())
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
			t.Fatal(err)
		}
		if len(obj) != 3 || obj["profile"] == nil || obj["catalog"] == nil || obj["lavxBalance"] == nil {
			t.Fatalf("wrong own state %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), `"username":"StaleName"`) || strings.Contains(w.Body.String(), `"isAdmin":true`) {
			t.Fatal("JWT role/name leaked into profile")
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("sensitive response cached")
		}
	}
	if s.player != "123456" || s.update.ShowBalance || s.update.Username != "NewName" {
		t.Fatal("wrong immutable identity or privacy input")
	}
	w := perform(mux, token, "GET", "/profile/view?playerId=654321", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var obj map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &obj)
	if obj["playerId"] != "654321" || obj["profile"] != nil || obj["balance"] != nil || obj["lavxBalance"] != nil {
		t.Fatal("view was not redacted public profile")
	}
	w = perform(mux, token, "GET", "/community", "")
	_ = json.Unmarshal(w.Body.Bytes(), &obj)
	for _, key := range []string{"news", "friends", "incoming", "outgoing", "leaderboard"} {
		if obj[key] == nil {
			t.Fatal("null list", key)
		}
	}
	if obj["myRank"] != nil {
		t.Fatal("optout rank not null")
	}
	w = perform(mux, token, "GET", "/community/search?q=ab", "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("search not direct list")
	}
	w = perform(mux, token, "POST", "/community/friend", `{"playerId":"654321","action":"request"}`)
	if strings.TrimSpace(w.Body.String()) != `{"friendState":"outgoing"}` {
		t.Fatal("relationship response", w.Body.String())
	}
}

func TestHTTPAvatarUploadAndDelete(t *testing.T) {
	s := &stubService{}
	mux, token := testMux(t, s)
	body, _ := json.Marshal(map[string]string{"image": imageURL(t, "png", image.NewNRGBA(image.Rect(0, 0, 64, 64)))})
	w := perform(mux, token, "POST", "/profile/avatar", string(body))
	if w.Code != 200 || len(s.avatar) == 0 || len(s.thumb) == 0 {
		t.Fatal("avatar upload failed", w.Code, w.Body.String())
	}
	w = perform(mux, token, "DELETE", "/profile/avatar", "")
	if w.Code != 200 || s.avatar != nil || s.thumb != nil {
		t.Fatal("avatar delete failed")
	}
}

func TestHTTPErrorRedactionAndStatuses(t *testing.T) {
	s := &stubService{}
	mux, token := testMux(t, s)
	for _, tc := range []struct {
		err    error
		status int
	}{
		{profile_sql.ErrUnauthorized, 401}, {profile_sql.ErrForbidden, 403}, {profile_sql.ErrNotFound, 404},
		{profile_sql.ErrNotOwned, 403}, {profile_sql.ErrInsufficientLavx, 409}, {profile_sql.ErrTransition, 409},
		{errors.New("password_hash=secret; private SQL statement"), 500},
	} {
		s.err = tc.err
		w := perform(mux, token, "GET", "/profile", "")
		if w.Code != tc.status {
			t.Fatal(w.Code, tc.err)
		}
		if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "SQL statement") {
			t.Fatal("private database error leaked")
		}
	}
}
