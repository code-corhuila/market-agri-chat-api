package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"details"`
	TraceID string `json:"traceId"`
}

// testServer mounts /health plus a route that panics.
func testServer(t *testing.T, logs io.Writer) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return NewHandler(logger, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/chat/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	})
}

func do(h http.Handler, method, path, token string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the error envelope: %q", rec.Body.String())
	}
	if body.Details == nil {
		t.Fatalf("details must be present (an empty array), body %q", rec.Body.String())
	}
	if body.TraceID == "" || body.TraceID != rec.Header().Get("X-Correlation-Id") {
		t.Fatalf("traceId %q must equal X-Correlation-Id %q", body.TraceID, rec.Header().Get("X-Correlation-Id"))
	}
	return body
}

func TestHealthIsUp(t *testing.T) {
	h := testServer(t, io.Discard)
	rec := do(h, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"status":"UP"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type %q", rec.Header().Get("Content-Type"))
	}
}

func TestUnknownRouteAndWrongMethodUseTheEnvelope(t *testing.T) {
	h := testServer(t, io.Discard)
	rec := do(h, http.MethodGet, "/api/v1/chat/nothing", "")
	if body := decodeError(t, rec); rec.Code != 404 || body.Error != "NOT_FOUND" || body.Message != "El recurso solicitado no existe." {
		t.Fatalf("404: %d %+v", rec.Code, body)
	}
	rec = do(h, http.MethodPost, "/health", "")
	if body := decodeError(t, rec); rec.Code != 405 || body.Error != "METHOD_NOT_ALLOWED" || body.Message != "Método no permitido." {
		t.Fatalf("405: %d %+v", rec.Code, body)
	}
}

func TestPanicIs500WithANeutralMessage(t *testing.T) {
	var logs bytes.Buffer
	h := testServer(t, &logs)
	rec := do(h, http.MethodGet, "/api/v1/chat/boom", "")
	if body := decodeError(t, rec); rec.Code != 500 || body.Error != "INTERNAL_ERROR" || body.Message != "Ocurrió un error inesperado." {
		t.Fatalf("got %d %+v", rec.Code, body)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Fatal("the detail goes to the log")
	}
}

func TestCorrelationId(t *testing.T) {
	var logs bytes.Buffer
	h := testServer(t, &logs)

	rec := do(h, http.MethodGet, "/health", "", "X-Correlation-Id", "abc-123")
	if rec.Header().Get("X-Correlation-Id") != "abc-123" {
		t.Fatal("a safe received id is reused")
	}
	if !strings.Contains(logs.String(), `"correlationId":"abc-123"`) {
		t.Fatalf("the access log carries the id: %s", logs.String())
	}

	rec = do(h, http.MethodGet, "/health", "", "X-Correlation-Id", "bad id\nINJECTED")
	got := rec.Header().Get("X-Correlation-Id")
	if got == "" || strings.Contains(got, " ") || len(got) != 36 {
		t.Fatalf("an unsafe id is replaced by a new UUID, got %q", got)
	}
	if strings.Contains(logs.String(), "INJECTED") {
		t.Fatal("the rejected value never reaches the log")
	}

	if do(h, http.MethodGet, "/health", "").Header().Get("X-Correlation-Id") == "" {
		t.Fatal("an id is generated when none is received")
	}
}
