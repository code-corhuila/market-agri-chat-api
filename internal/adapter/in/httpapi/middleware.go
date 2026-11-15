package httpapi

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

// CorrelationHeader travels from the gateway to every service and back (Anexo C 5.3.9, D-C28).
const CorrelationHeader = "X-Correlation-Id"

// safeID: a received id is reused only if it is safe to log (no spaces, no line breaks).
var safeID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type correlationKey struct{}
type loggerKey struct{}

// CorrelationID returns the id of the request (the error traceId).
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}

// loggerFrom returns the request logger, which carries correlationId on every line.
func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// LoggerFrom is the request logger for use cases' adapters and handlers.
func LoggerFrom(ctx context.Context) *slog.Logger { return loggerFrom(ctx) }

// NewHandler mounts the routes on a mux with /health and wraps them in this order: correlation,
// access log, recovery, and the error envelope for an unknown route or a wrong method.
func NewHandler(logger *slog.Logger, routes func(mux *http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "UP"}) // liveness (E-60, D-C20)
	})
	routes(mux)
	return correlation(logger, accessLog(recovery(envelope(mux))))
}

func correlation(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received := r.Header.Get(CorrelationHeader)
		id := received
		if !safeID.MatchString(received) {
			id = newUUID()
		}
		w.Header().Set(CorrelationHeader, id)
		log := logger.With("correlationId", id)
		if received != "" && received != id {
			// Only the length: the rejected value itself is what must never reach the log.
			log.Warn(fmt.Sprintf("Rejected unsafe X-Correlation-Id of %d characters; generated a new one", len(received)))
		}
		ctx := context.WithValue(context.WithValue(r.Context(), correlationKey{}, id), loggerKey{}, log)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		loggerFrom(r.Context()).Info("request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "durationMs", time.Since(start).Milliseconds())
	})
}

// recovery turns a panic into 500 with the neutral message; the detail goes only to the log (NFR-039).
func recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				loggerFrom(r.Context()).Error("Unhandled panic", "panic", fmt.Sprint(p))
				WriteError(w, r, CodeInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// envelope answers an unknown route (404) or a wrong method (405, with its Allow header) with the
// contract's body instead of the mux's plain text.
func envelope(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		probe := &headerOnly{header: http.Header{}}
		handler.ServeHTTP(probe, r)
		if probe.status == http.StatusMethodNotAllowed {
			w.Header().Set("Allow", probe.header.Get("Allow"))
			WriteError(w, r, CodeMethodNotAllowed)
			return
		}
		WriteError(w, r, CodeNotFound)
	})
}

// headerOnly captures the status and headers the mux's own 404/405 handler would write.
type headerOnly struct {
	header http.Header
	status int
}

func (h *headerOnly) Header() http.Header         { return h.header }
func (h *headerOnly) Write(b []byte) (int, error) { return len(b), nil }
func (h *headerOnly) WriteHeader(code int)        { h.status = code }

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
