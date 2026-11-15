package httpapi

import (
	"context"
	"net/http"
)

// readiness is the body of E-61, with status first as in the Java services.
type readiness struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ReadinessRoute is E-61 (D-C20): 200 when the database answers, 503 otherwise, with the same body.
// Liveness (/health) checks nothing but the process, so a database outage never restarts chat-api.
func ReadinessRoute(pingDatabase func(context.Context) error) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
			status, code := "UP", http.StatusOK
			if err := pingDatabase(r.Context()); err != nil {
				loggerFrom(r.Context()).Warn("Readiness: database down", "error", err.Error())
				status, code = "DOWN", http.StatusServiceUnavailable
			}
			WriteJSON(w, code, readiness{Status: status, Checks: map[string]string{"database": status}})
		})
	}
}
