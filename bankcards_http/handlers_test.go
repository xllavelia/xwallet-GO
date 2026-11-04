package bankcards_http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
)

type userContext struct{ context.Context }

func (ctx userContext) Value(key interface{}) interface{} {
	if fmt.Sprint(key) == "authUser" {
		return auth_http.AuthenticatedUser{PlayerID: "test-player"}
	}
	return ctx.Context.Value(key)
}

func TestOpenValidationJSON(t *testing.T) {
	for _, body := range []string{
		`{"tier":"saint"}`,
		`{"name":"","designId":5}`,
		`{"name":"Name","designId":4}`,
		`{"name":"Name","designId":10}`,
		`{"name":"Name","designId":5.5}`,
		`{"name":"a\nb","designId":5}`,
		`{"name":"Name","designId":5} {}`,
		`{"name":`,
	} {
		r := httptest.NewRequest(http.MethodPost, "/bankcards/open", strings.NewReader(body))
		r = r.WithContext(userContext{r.Context()})
		w := httptest.NewRecorder()
		OpenHandler(nil)(w, r)
		assertJSONError(t, w, http.StatusBadRequest)
	}
}

func TestFundingBoundaryValidationJSON(t *testing.T) {
	for _, handler := range []http.HandlerFunc{TopUpHandler(nil), WithdrawHandler(nil)} {
		for _, amount := range []string{"0", "-1", "0.001", "1.234", "1e309", "1000000000000"} {
			r := httptest.NewRequest(http.MethodPost, "/bankcards/topup", strings.NewReader(`{"cardId":42,"amount":`+amount+`}`))
			r = r.WithContext(userContext{r.Context()})
			w := httptest.NewRecorder()
			handler(w, r)
			assertJSONError(t, w, http.StatusBadRequest)
		}
	}
}

func TestAccountConflictErrors(t *testing.T) {
	for _, err := range []error{bankcards_sql.ErrMaxCardsReached, bankcards_sql.ErrDesignAlreadyOwned, bankcards_sql.ErrCardFunded, bankcards_sql.ErrCardHasOpenPositions} {
		w := httptest.NewRecorder()
		writeCardError(w, err, "failed")
		assertJSONError(t, w, http.StatusConflict)
		var body map[string]string
		if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil || body["error"] != err.Error() {
			t.Fatalf("message not preserved: %s", w.Body.String())
		}
	}
}

func TestAccountResponseContract(t *testing.T) {
	card := bankcards_sql.Card{
		ID: 42, UserID: 7, Tier: "standard", Name: "Savings", DesignID: 9,
		CardNumber: "1234567890123456", Balance: 12.34, IsActiveForTrading: true,
		OpenedAt: time.Date(2026, 11, 2, 15, 0, 0, 0, time.FixedZone("test", 3*60*60)),
	}
	encoded, err := json.Marshal(responseCard(card))
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"balance", "cardNumber", "designId", "id", "isActiveForTrading", "name", "openedAt"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("card keys: %v", keys)
	}
	if data["openedAt"] != "2026-11-02T12:00:00Z" || data["name"] != "Savings" || data["balance"] != 12.34 || data["isActiveForTrading"] != true {
		t.Fatalf("card data lost: %s", encoded)
	}
	encoded, err = json.Marshal(listResponse{Cards: []cardResponse{}, MaxCards: bankcards_sql.MaxCardsPerUser})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"cards":[],"maxCards":5}` {
		t.Fatalf("list contract: %s", encoded)
	}
}

func TestRequestBodyBounded(t *testing.T) {
	for _, body := range []string{`{"name":"` + strings.Repeat("x", 5000) + `","designId":5}`, `{"name":"Name","designId":5,"price":0}`, `{"name":"Name","designId":5} junk`} {
		r := httptest.NewRequest(http.MethodPost, "/bankcards/open", strings.NewReader(body))
		if err := decodeRequest(httptest.NewRecorder(), r, &openRequest{}); err == nil {
			t.Fatal("invalid body accepted")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/bankcards/open", strings.NewReader(`{"name":"旅行","designId":5}`))
	var req openRequest
	if err := decodeRequest(httptest.NewRecorder(), r, &req); err != nil || req.Name != "旅行" || req.DesignID != 5 {
		t.Fatalf("valid body: %+v, %v", req, err)
	}
}

func assertJSONError(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status/content type: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("invalid JSON error: %s", w.Body.String())
	}
}
