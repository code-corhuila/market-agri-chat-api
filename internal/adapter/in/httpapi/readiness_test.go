package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestReadinessReportsTheDatabase(t *testing.T) {
	for _, c := range []struct {
		name   string
		ping   error
		status int
		body   string
	}{
		{"database up", nil, 200, `{"status":"UP","checks":{"database":"UP"}}`},
		{"database down", errors.New("no primary"), 503, `{"status":"DOWN","checks":{"database":"DOWN"}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)),
				ReadinessRoute(func(context.Context) error { return c.ping }))
			rec := do(h, http.MethodGet, "/health/ready", "")
			if rec.Code != c.status || strings.TrimSpace(rec.Body.String()) != c.body {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
