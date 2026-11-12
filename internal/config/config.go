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

	ReadHeaderTimeout time.Duration // slow clients cannot hold a connection open
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64         // JSON bodies of E-30 and E-34 are tiny
	ShutdownTimeout   time.Duration // graceful shutdown, shorter than the compose stop_grace_period
}

// Load reads the environment. JWT_ISSUER and JWT_PUBLIC_KEY (or JWT_PUBLIC_KEY_FILE) are required.
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
