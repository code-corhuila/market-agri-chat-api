package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

// authServer mounts three probe routes that exercise the token rules: a user route for BUYER only,
// a user route for any user role and an internal route for services.
func authServer(t *testing.T) (http.Handler, testKeys, time.Time) {
	t.Helper()
	keys := newTestKeys(t)
	verifier, err := NewVerifier(keys.publicPEMOneLine(t), testIssuer, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	auth := Auth{Verifier: verifier}
	h := NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/chat/probe", auth.User([]string{"BUYER"}, func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, map[string]string{"userId": CallerFrom(r.Context()).UserID})
		}))
		mux.HandleFunc("GET /api/v1/chat/any-user", auth.User(nil, func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, map[string]string{"role": CallerFrom(r.Context()).Role})
		}))
		mux.HandleFunc("GET /internal/v1/chat/probe", auth.Service(func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, map[string]string{"sub": CallerFrom(r.Context()).UserID})
		}))
	})
	return h, keys, time.Now()
}

func TestTokenRules(t *testing.T) {
	h, keys, now := authServer(t)
	buyer := sign(t, keys.private, rs256(), userClaims(now))
	producerClaims := userClaims(now)
	producerClaims["role"] = "PRODUCER"
	producer := sign(t, keys.private, rs256(), producerClaims)
	service := sign(t, keys.private, rs256(), serviceClaims(now))

	cases := []struct {
		name, path, token string
		status            int
		code              string
	}{
		{"a valid user token passes", "/api/v1/chat/probe", buyer, 200, ""},
		{"no token is 401", "/api/v1/chat/probe", "", 401, "UNAUTHORIZED"},
		{"an invalid token is 401", "/api/v1/chat/probe", "x.y.z", 401, "UNAUTHORIZED"},
		{"a role the route does not take is 403", "/api/v1/chat/probe", producer, 403, "FORBIDDEN"},
		{"a service token on a public route is 403", "/api/v1/chat/any-user", service, 403, "FORBIDDEN"},
		{"any user role passes a route without a role list", "/api/v1/chat/any-user", producer, 200, ""},
		{"a user token on an internal route is 403", "/internal/v1/chat/probe", buyer, 403, "FORBIDDEN"},
		{"a service token on an internal route passes", "/internal/v1/chat/probe", service, 200, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(h, http.MethodGet, c.path, c.token)
			if rec.Code != c.status {
				t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
			}
			if c.code != "" {
				if body := decodeError(t, rec); body.Error != c.code {
					t.Fatalf("error %q", body.Error)
				}
			}
			if c.status == 401 && rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("401 carries WWW-Authenticate: Bearer")
			}
		})
	}

	rec := do(h, http.MethodGet, "/api/v1/chat/probe", buyer)
	if !strings.Contains(rec.Body.String(), "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d") {
		t.Fatal("the handler reads the caller of the token")
	}
	rec = do(h, http.MethodGet, "/api/v1/chat/probe", "", "Authorization", "Basic abc")
	if rec.Code != 401 {
		t.Fatal("a header that is not Bearer is 401")
	}
}
