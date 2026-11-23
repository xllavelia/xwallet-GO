package bankcards_http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xwallet-server/auth_http"
	"xwallet-server/bankcards_sql"
)

func decodeDesignBody(body string, dst any) error {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	return decodeRequestLimit(httptest.NewRecorder(), r, dst, 512*1024)
}

func TestStrictCustomRequestSchemas(t *testing.T) {
	for _, request := range []struct {
		name   string
		prefix string
		newDst func() any
	}{
		{"open", `"name":"Savings",`, func() any { return &openRequest{} }},
		{"design", `"cardId":1,`, func() any { return &designRequest{} }},
		{"charge", `"voucherId":0,`, func() any { return &customChargeRequest{} }},
	} {
		t.Run(request.name, func(t *testing.T) {
			for _, custom := range []string{
				`{"nft":{"itemId":1}}`, `{"left":{"color":"#fff"}}`, `{"right":{}}`,
				`{"color":"#fff"}`, `{"colors":["#fff"]}`, `{"image":"photo"}`, `{"photos":[]}`,
				`{"ink":"#fff"}`, `{"inkOpacity":1}`, `{"link":true}`, `{"photo":"photo","right":{}}`,
				`{"designKey":"card-sage","nft":null}`, `{"designKey":"card-sage","price":0}`,
			} {
				body := "{" + request.prefix + `"custom":` + custom + "}"
				if err := decodeDesignBody(body, request.newDst()); err == nil {
					t.Fatalf("legacy/unknown custom field accepted: %s", body)
				}
			}
			for _, custom := range []string{`{"designKey":"card-sage"}`, `{"photo":"data:image/jpeg;base64,/9j/"}`} {
				body := "{" + request.prefix + `"custom":` + custom + "}"
				if err := decodeDesignBody(body, request.newDst()); err != nil {
					t.Fatalf("new custom schema failed to decode: %s %v", body, err)
				}
			}
			for _, extra := range []string{`"nft":null`, `"colors":[]`, `"right":{}`, `"price":0`} {
				body := "{" + request.prefix + `"custom":{"designKey":"card-sage"},` + extra + "}"
				if err := decodeDesignBody(body, request.newDst()); err == nil {
					t.Fatalf("unknown outer field accepted: %s", body)
				}
			}
		})
	}
	for _, body := range []string{
		`{"voucherId":0,"designKey":"card-sage"}`, `{"voucherId":0,"designId":0}`,
		`{"voucherId":0,"custom":{"designKey":"card-sage"}} {}`,
		`{"voucherId":"0","custom":{"designKey":"card-sage"}}`,
		`{"voucherId":0,"custom":{"photo":123}}`, "[]",
	} {
		if err := decodeDesignBody(body, &customChargeRequest{}); err == nil {
			t.Fatalf("invalid charge request decoded: %s", body)
		}
	}
}

func TestRequestDesignExclusivity(t *testing.T) {
	for _, body := range []string{
		`{"designId":0,"designKey":"card-sage"}`, `{"designId":0,"custom":{"designKey":"card-sage"}}`,
		`{"designId":1,"custom":{"designKey":"card-sage"}}`,
		`{"designKey":"card-sage","custom":{"designKey":"card-silk"}}`,
		`{"designId":0,"designKey":"card-sage","custom":{"designKey":"card-silk"}}`,
	} {
		for _, open := range []bool{false, true} {
			var designID *int
			var designKey string
			var custom *bankcards_sql.CardCustomDesign
			if open {
				var req openRequest
				if err := decodeDesignBody(body, &req); err != nil {
					t.Fatal(err)
				}
				designID, designKey, custom = req.DesignID, req.DesignKey, req.Custom
			} else {
				var req designRequest
				if err := decodeDesignBody(body, &req); err != nil {
					t.Fatal(err)
				}
				designID, designKey, custom = req.DesignID, req.DesignKey, req.Custom
			}
			choice := designChoiceFromRequest(designID, designKey, custom)
			if _, err := bankcards_sql.ValidateDesignChoice(choice, nil, true); !errors.Is(err, bankcards_sql.ErrInvalidDesign) {
				t.Fatalf("conflicting request accepted open=%v: %s %v", open, body, err)
			}
		}
	}
	for _, body := range []string{`{}`, `{"designId":0}`, `{"designId":3}`, `{"designKey":"card-gold"}`, `{"custom":{"designKey":"card-sage"}}`} {
		var req openRequest
		if err := decodeDesignBody(body, &req); err != nil {
			t.Fatal(err)
		}
		choice := designChoiceFromRequest(req.DesignID, req.DesignKey, req.Custom)
		if _, err := bankcards_sql.ValidateDesignChoice(choice, map[string]bool{"card-gold": true}, false); err != nil {
			t.Fatalf("valid single selection rejected: %s %v", body, err)
		}
	}
}

func TestDesignRequestBodyLimits(t *testing.T) {
	body := `{"voucherId":0,"custom":{"designKey":"card-sage"}}`
	for _, size := range []int{512 * 1024, 512*1024 + 1} {
		padded := body + strings.Repeat(" ", size-len(body))
		err := decodeDesignBody(padded, &customChargeRequest{})
		if size == 512*1024 && err != nil {
			t.Fatalf("exact request limit rejected: %v", err)
		}
		if size > 512*1024 && err == nil {
			t.Fatal("oversized request accepted")
		}
	}
	photoBody := `{"voucherId":7,"custom":{"photo":"data:image/jpeg;base64,` + strings.Repeat("A", 220000) + `"}}`
	var req customChargeRequest
	if err := decodeDesignBody(photoBody, &req); err != nil || req.Custom == nil || req.VoucherID != 7 {
		t.Fatalf("photo charge request still uses old 4KB limit: %v", err)
	}
}

// Контекст авторизации задаётся напрямую: тесты не читают секреты и не обращаются к БД.
type authenticatedContext struct{ context.Context }

func (c authenticatedContext) Value(key any) any {
	if fmt.Sprint(key) == "authUser" {
		return auth_http.AuthenticatedUser{PlayerID: "123456", IsAdmin: true}
	}
	return c.Context.Value(key)
}

func TestCustomChargeHandlerRejectsBeforeDatabase(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"voucherId":0}`, `{"voucherId":0,"custom":null}`, `{"voucherId":-1,"custom":{"designKey":"card-sage"}}`,
		`{"custom":{}}`, `{"custom":{"designKey":"card-gold"}}`, `{"custom":{"designKey":"card-dusk"}}`,
		`{"custom":{"designKey":"card-nebula"}}`, `{"custom":{"designKey":"card-xlavelia"}}`,
		`{"custom":{"photo":"data:image/jpeg;base64,AAAA"}}`,
		`{"custom":{"designKey":"card-sage","photo":"photo"}}`,
		`{"custom":{"nft":{"itemId":1}}}`, strings.Repeat(" ", 512*1024+1),
	} {
		r := httptest.NewRequest(http.MethodPost, "/bankcards/custom/charge", strings.NewReader(body))
		r = r.WithContext(authenticatedContext{context.Background()})
		w := httptest.NewRecorder()
		CustomChargeHandler(nil)(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid charge request reached database or wrong status (%d bytes): %d %s", len(body), w.Code, w.Body.String())
		}
	}
}

func TestDesignHandlersMethodsAndAuth(t *testing.T) {
	for _, handler := range []http.HandlerFunc{OpenHandler(nil), DesignHandler(nil), CustomChargeHandler(nil)} {
		for _, tc := range []struct {
			method string
			status int
		}{{http.MethodGet, http.StatusMethodNotAllowed}, {http.MethodPost, http.StatusUnauthorized}} {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tc.status {
				t.Fatalf("method/auth status: %d want %d", w.Code, tc.status)
			}
		}
	}
}

func TestDesignErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{bankcards_sql.ErrCustomInvalid, http.StatusBadRequest}, {bankcards_sql.ErrInvalidDesign, http.StatusBadRequest},
		{bankcards_sql.ErrVoucherInvalid, http.StatusBadRequest}, {bankcards_sql.ErrDesignNotOwned, http.StatusForbidden},
		{bankcards_sql.ErrInsufficientFunds, http.StatusPaymentRequired}, {bankcards_sql.ErrCardNotFound, http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		writeCardError(w, fmt.Errorf("wrapped: %w", tc.err), "fallback")
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || body["error"] != "wrapped: "+tc.err.Error() || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("error mapping changed: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestCardResponseCustomSchema(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	for _, custom := range []*bankcards_sql.CardCustomDesign{
		{DesignKey: "card-sage"}, {Photo: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())},
	} {
		raw, err := json.Marshal(responseCard(bankcards_sql.Card{Custom: custom}))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Custom map[string]json.RawMessage `json:"custom"`
		}
		if err := json.Unmarshal(raw, &body); err != nil || len(body.Custom) != 1 {
			t.Fatalf("response exposes legacy custom fields: %s %v", raw, err)
		}
	}
	raw, err := json.Marshal(responseCard(bankcards_sql.Card{}))
	if err != nil || strings.Contains(string(raw), `"custom"`) {
		t.Fatalf("default response must omit custom: %s %v", raw, err)
	}
}
