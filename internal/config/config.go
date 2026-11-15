// Package config is the composition root's configuration: every limit of the service is declared
// here with its value (Anexo C 5.3.10), and the required variables stop the start when missing.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Config of chat-api, read only from the environment.
type Config struct {
	Port         string
	JWTIssuer    string
	JWTPublicKey string

	MongoURL      string // as chat_app, replicaSet=rs0 (market-agri-infra-mongo)
	MongoDatabase string
	CatalogAPIURL string // I-02
	ServiceToken  string // the token of the client chat, issued by I-01 (ADR-020 point 5)

	ReadHeaderTimeout time.Duration // slow clients cannot hold a connection open
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64         // JSON bodies of E-30 and E-34 are tiny
	ShutdownTimeout   time.Duration // graceful shutdown, shorter than the compose stop_grace_period

	MongoServerSelection time.Duration // how long a query waits for the primary
	MongoMaxPool         uint64
	PingTimeout          time.Duration // readiness
	HTTPTimeout          time.Duration // per attempt of an outgoing call (Anexo D)
	HTTPAttempts         int
	HTTPBaseDelay        time.Duration // full jitter base of the retries
}

// Load reads the environment. JWT_ISSUER, JWT_PUBLIC_KEY (or JWT_PUBLIC_KEY_FILE), MONGO_URL,
// CATALOG_API_URL and SERVICE_TOKEN are required.
func Load() (Config, error) {
	c := Config{
		Port:              env("PORT", "8080"),
		JWTIssuer:         os.Getenv("JWT_ISSUER"),
		JWTPublicKey:      os.Getenv("JWT_PUBLIC_KEY"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		MaxBodyBytes:      16 << 10,
		ShutdownTimeout:   20 * time.Second,

		MongoURL:             os.Getenv("MONGO_URL"),
		MongoDatabase:        "chat",
		CatalogAPIURL:        os.Getenv("CATALOG_API_URL"),
		ServiceToken:         os.Getenv("SERVICE_TOKEN"),
		MongoServerSelection: 5 * time.Second,
		MongoMaxPool:         20,
		PingTimeout:          2 * time.Second,
		HTTPTimeout:          3 * time.Second,
		HTTPAttempts:         3,
		HTTPBaseDelay:        200 * time.Millisecond,
	}
	if file := os.Getenv("JWT_PUBLIC_KEY_FILE"); c.JWTPublicKey == "" && file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return Config{}, err
		}
		c.JWTPublicKey = string(raw)
	}
	var missing []string
	if strings.TrimSpace(c.JWTIssuer) == "" {
		missing = append(missing, "JWT_ISSUER")
	}
	if strings.TrimSpace(c.JWTPublicKey) == "" {
		missing = append(missing, "JWT_PUBLIC_KEY")
	}
	for _, v := range []struct{ name, value string }{
		{"MONGO_URL", c.MongoURL}, {"CATALOG_API_URL", c.CatalogAPIURL}, {"SERVICE_TOKEN", c.ServiceToken},
	} { // a slice, not a map: the message lists the variables always in the same order
		if strings.TrimSpace(v.value) == "" {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		return Config{}, errors.New("missing required variables: " + strings.Join(missing, ", "))
	}
	return c, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
