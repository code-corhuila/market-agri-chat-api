package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/adapter/out/persistence"
	"github.com/code-corhuila/market-agri-chat-api/internal/application/usecase"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

const (
	moraID     = "5e1f0c2a-7b3d-4e8f-9a6b-000000000021"
	producerID = "0b6f2c7e-4a51-4c1e-9d0e-2f1c3a7b9e10"
	buyerID    = "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d"
)

type stubCatalog struct{ err error }

func (s stubCatalog) Product(_ context.Context, id string) (model.ProductSnapshot, error) {
	if s.err != nil {
		return model.ProductSnapshot{}, s.err
	}
	if id != moraID {
		return model.ProductSnapshot{}, model.ErrProductNotFound
	}
	return model.ProductSnapshot{ID: moraID, Name: "Mora de Castilla", ProducerID: producerID, ProducerName: "Ana Ruiz"}, nil
}

type chatFixture struct {
	h                         http.Handler
	buyer, producer, stranger string
	service                   string
}

func newChatFixture(t *testing.T, catalog stubCatalog) chatFixture {
	t.Helper()
	keys := newTestKeys(t)
	verifier, err := NewVerifier(keys.publicPEMOneLine(t), testIssuer, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	conversations := &usecase.Conversations{
		Store: persistence.NewMemory(), Catalog: catalog,
		NewID: func() string { return "7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e" },
		Now:   func() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) },
	}
	h := NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		ConversationRoutes(Auth{Verifier: verifier}, conversations, 16<<10))
	now := time.Now()
	token := func(sub, role, name string) string {
		c := userClaims(now)
		c["sub"], c["role"], c["name"] = sub, role, name
		return sign(t, keys.private, rs256(), c)
	}
	return chatFixture{
		h:        h,
		buyer:    token(buyerID, "BUYER", "Carlos Pérez"),
		producer: token(producerID, "PRODUCER", "Ana Ruiz"),
		stranger: token("d4444444-4444-4444-8444-444444444444", "BUYER", "Otro"),
		service:  sign(t, keys.private, rs256(), serviceClaims(now)),
	}
}

func call(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func view(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	return v
}

func TestE30OpensThenReturnsTheSame(t *testing.T) {
	f := newChatFixture(t, stubCatalog{})
	rec := call(f.h, http.MethodPost, "/api/v1/chat/conversations", f.buyer, `{"productId":"`+moraID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/chat/conversations/7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e" {
		t.Fatalf("Location %q", rec.Header().Get("Location"))
	}
	v := view(t, rec)
	want := map[string]any{
		"id": "7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e", "productId": moraID, "productName": "Mora de Castilla",
		"buyerId": buyerID, "buyerName": "Carlos Pérez", "producerId": producerID, "producerName": "Ana Ruiz",
		"agreedPurchaseMethod": nil, "lastMessageAt": nil, "createdAt": "2026-11-12T10:00:00Z",
	}
	for k, w := range want {
		if got, ok := v[k]; !ok || got != w {
			t.Errorf("%s = %v (present %v), want %v", k, got, ok, w)
		}
	}
	if len(v) != len(want) {
		t.Errorf("ConversationView has exactly %d fields, got %v", len(want), v)
	}

	again := call(f.h, http.MethodPost, "/api/v1/chat/conversations", f.buyer, `{"productId":"`+moraID+`"}`)
	if again.Code != http.StatusOK || view(t, again)["id"] != v["id"] {
		t.Fatalf("reopen: %d %s", again.Code, again.Body.String())
	}
}

func TestE30Errors(t *testing.T) {
	f := newChatFixture(t, stubCatalog{})
	cases := []struct {
		name, token, body string
		status            int
		code, message     string
	}{
		{"a malformed productId is 400 on productId", f.buyer, `{"productId":"42"}`, 400, "VALIDATION_ERROR", "La solicitud tiene datos inválidos."},
		{"a missing productId is 400", f.buyer, `{}`, 400, "VALIDATION_ERROR", "La solicitud tiene datos inválidos."},
		{"a body that is not JSON is 400", f.buyer, `{`, 400, "VALIDATION_ERROR", "La solicitud tiene datos inválidos."},
		{"an unknown product is 404", f.buyer, `{"productId":"5e1f0c2a-7b3d-4e8f-9a6b-000000000099"}`, 404, "PRODUCT_NOT_FOUND", "El producto no existe."},
		{"a producer cannot open one", f.producer, `{"productId":"` + moraID + `"}`, 403, "FORBIDDEN", "No tiene permisos para realizar esta acción."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := call(f.h, http.MethodPost, "/api/v1/chat/conversations", c.token, c.body)
			v := view(t, rec)
			if rec.Code != c.status || v["error"] != c.code || v["message"] != c.message {
				t.Fatalf("got %d %v", rec.Code, v)
			}
		})
	}
	rec := call(f.h, http.MethodPost, "/api/v1/chat/conversations", f.buyer, `{"productId":"42"}`)
	if !strings.Contains(rec.Body.String(), `"field":"productId"`) {
		t.Fatalf("details name the field: %s", rec.Body.String())
	}

	down := newChatFixture(t, stubCatalog{err: model.ErrCatalogUnavailable})
	rec = call(down.h, http.MethodPost, "/api/v1/chat/conversations", down.buyer, `{"productId":"`+moraID+`"}`)
	if v := view(t, rec); rec.Code != 503 || v["error"] != "CATALOG_UNAVAILABLE" || v["message"] != "El catálogo no está disponible. Intente de nuevo." {
		t.Fatalf("catalog down: %d %v", rec.Code, v)
	}
}

func TestE32AndI05(t *testing.T) {
	f := newChatFixture(t, stubCatalog{})
	call(f.h, http.MethodPost, "/api/v1/chat/conversations", f.buyer, `{"productId":"`+moraID+`"}`)
	path := "/api/v1/chat/conversations/7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e"

	if rec := call(f.h, http.MethodGet, path, f.producer, ""); rec.Code != 200 || view(t, rec)["buyerName"] != "Carlos Pérez" {
		t.Fatalf("the producer reads it: %d", rec.Code)
	}
	rec := call(f.h, http.MethodGet, path, f.stranger, "")
	if v := view(t, rec); rec.Code != 403 || v["error"] != "NOT_A_PARTICIPANT" || v["message"] != "No participa en esta conversación." {
		t.Fatalf("stranger: %d %v", rec.Code, v)
	}
	for _, missing := range []string{"/api/v1/chat/conversations/7e0b1d2c-3a4f-4b5c-8d6e-000000000000", "/api/v1/chat/conversations/bad"} {
		rec = call(f.h, http.MethodGet, missing, f.stranger, "")
		if v := view(t, rec); rec.Code != 404 || v["error"] != "CONVERSATION_NOT_FOUND" || v["message"] != "La conversación no existe." {
			t.Fatalf("%s: %d %v", missing, rec.Code, v)
		}
	}

	internal := "/internal/v1/chat/conversations/7e0b1d2c-3a4f-4b5c-8d6e-9f0a1b2c3d4e"
	if rec := call(f.h, http.MethodGet, internal, f.service, ""); rec.Code != 200 || view(t, rec)["producerId"] != producerID {
		t.Fatalf("I-05: %d", rec.Code)
	}
	if rec := call(f.h, http.MethodGet, "/internal/v1/chat/conversations/7e0b1d2c-3a4f-4b5c-8d6e-000000000000", f.service, ""); rec.Code != 404 {
		t.Fatalf("I-05 missing: %d", rec.Code)
	}
	if rec := call(f.h, http.MethodGet, internal, f.buyer, ""); rec.Code != 403 {
		t.Fatalf("I-05 with a user token: %d", rec.Code)
	}
}

func TestE31(t *testing.T) {
	f := newChatFixture(t, stubCatalog{})
	call(f.h, http.MethodPost, "/api/v1/chat/conversations", f.buyer, `{"productId":"`+moraID+`"}`)

	rec := call(f.h, http.MethodGet, "/api/v1/chat/conversations", f.producer, "")
	var page struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(page.Data) != 1 || page.Meta["page"] != 1.0 || page.Meta["limit"] != 20.0 || page.Meta["total"] != 1.0 || page.Meta["totalPages"] != 1.0 {
		t.Fatalf("page %+v", page)
	}
	rec = call(f.h, http.MethodGet, "/api/v1/chat/conversations", f.stranger, "")
	if !strings.Contains(rec.Body.String(), `"data":[]`) || !strings.Contains(rec.Body.String(), `"totalPages":0`) {
		t.Fatalf("a stranger sees an empty page: %s", rec.Body.String())
	}
	for _, q := range []string{"?page=0", "?limit=101", "?limit=0", "?page=x", "?other=1"} {
		if rec := call(f.h, http.MethodGet, "/api/v1/chat/conversations"+q, f.buyer, ""); rec.Code != 400 {
			t.Fatalf("%s must be 400, got %d", q, rec.Code)
		}
	}
}
