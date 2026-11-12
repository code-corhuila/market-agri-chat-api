package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

const moraID = "5e1f0c2a-7b3d-4e8f-9a6b-000000000021"

func client(url string) *Client {
	return &Client{
		BaseURL: url, ServiceToken: "svc-token", Timeout: 200 * time.Millisecond, Attempts: 3, BaseDelay: time.Millisecond,
		HTTP: &http.Client{}, CorrelationID: func(context.Context) string { return "corr-1" },
	}
}

func TestProductReadsI02WithTheServiceToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/catalog/products/"+moraID || r.Header.Get("Authorization") != "Bearer svc-token" ||
			r.Header.Get("X-Correlation-Id") != "corr-1" {
			t.Errorf("request %s %v", r.URL.Path, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + moraID + `","producerId":"0b6f2c7e-4a51-4c1e-9d0e-2f1c3a7b9e10","producerName":"Ana Ruiz","name":"Mora de Castilla","status":"ACTIVE","priceCents":850000}`))
	}))
	defer srv.Close()
	p, err := client(srv.URL).Product(context.Background(), moraID)
	if err != nil {
		t.Fatal(err)
	}
	want := model.ProductSnapshot{ID: moraID, Name: "Mora de Castilla", ProducerID: "0b6f2c7e-4a51-4c1e-9d0e-2f1c3a7b9e10", ProducerName: "Ana Ruiz"}
	if p != want {
		t.Fatalf("got %+v", p)
	}
}

func TestProductNotFoundIsFinal(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := client(srv.URL).Product(context.Background(), moraID); !errors.Is(err, model.ErrProductNotFound) {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("a 404 is an answer, not retried: %d calls", calls.Load())
	}
}

func TestTransientFailuresAreRetriedThenUnavailable(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := client(srv.URL).Product(context.Background(), moraID); !errors.Is(err, model.ErrCatalogUnavailable) {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("5xx is retried up to 3 attempts: %d", calls.Load())
	}
}

func TestARetryCanSucceed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + moraID + `","producerId":"p","producerName":"Ana","name":"Mora"}`))
	}))
	defer srv.Close()
	if _, err := client(srv.URL).Product(context.Background(), moraID); err != nil || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestTimeoutAndRefusalAreUnavailable(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond) // a valid answer, but after the 200 ms of each attempt
		_, _ = w.Write([]byte(`{"id":"` + moraID + `","producerId":"p","producerName":"Ana","name":"Mora"}`))
	}))
	defer slow.Close()
	if _, err := client(slow.URL).Product(context.Background(), moraID); !errors.Is(err, model.ErrCatalogUnavailable) {
		t.Fatalf("timeout: %v", err)
	}
	var calls atomic.Int32
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer refused.Close()
	if _, err := client(refused.URL).Product(context.Background(), moraID); !errors.Is(err, model.ErrCatalogUnavailable) || calls.Load() != 1 {
		t.Fatalf("a 401 (expired service token) is not retried and is unavailable: %v, %d calls", err, calls.Load())
	}
}

// failFirst fails the first request at the network level, then lets the others through.
type failFirst struct{ calls atomic.Int32 }

func (f *failFirst) RoundTrip(r *http.Request) (*http.Response, error) {
	if f.calls.Add(1) == 1 {
		return nil, errors.New("connection reset")
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestANetworkFailureIsRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + moraID + `","producerId":"p","producerName":"Ana","name":"Mora"}`))
	}))
	defer srv.Close()
	transport := &failFirst{}
	c := client(srv.URL)
	c.HTTP = &http.Client{Transport: transport}
	if _, err := c.Product(context.Background(), moraID); err != nil || transport.calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, transport.calls.Load())
	}
}

func TestAnUnreadableAnswerIsUnavailableAndFinal(t *testing.T) {
	for _, body := range []string{`{}`, `not json`, `{"id":"` + moraID + `"}`} {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			_, _ = w.Write([]byte(body))
		}))
		_, err := client(srv.URL).Product(context.Background(), moraID)
		srv.Close()
		if !errors.Is(err, model.ErrCatalogUnavailable) || calls.Load() != 1 {
			t.Fatalf("%s: %v, %d calls", strings.TrimSpace(body), err, calls.Load())
		}
	}
}
