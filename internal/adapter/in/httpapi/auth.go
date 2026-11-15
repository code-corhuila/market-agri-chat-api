package httpapi

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// clockSkew tolerated on exp, as in the Java services.
const clockSkew = 30 * time.Second

// RoleService is the role of a service token (I-01); it is accepted only on /internal/** (D-C27).
const RoleService = "SERVICE"

var knownRoles = map[string]bool{"PRODUCER": true, "BUYER": true, "ADMIN": true, RoleService: true}

// Caller is who a valid token says is calling. For a service token, UserID is "service:<client>".
type Caller struct {
	UserID string
	Role   string
	Name   string
}

// Verifier validates an access or service token by itself, with the public key of auth (Anexo C
// 5.3.7, ADR-020). Closed list of algorithms: only RS256, so "none" and HS256 signed with the
// public key never pass. Standard library only, like the Java Rs256Verifier.
type Verifier struct {
	key    *rsa.PublicKey
	issuer string
	now    func() time.Time
}

// NewVerifier reads JWT_PUBLIC_KEY: a PEM on one line with literal \n (or real line breaks).
func NewVerifier(publicKeyPEM, issuer string, now func() time.Time) (*Verifier, error) {
	if strings.TrimSpace(issuer) == "" {
		return nil, errors.New("JWT_ISSUER is required")
	}
	block, _ := pem.Decode([]byte(strings.ReplaceAll(publicKeyPEM, `\n`, "\n")))
	if block == nil {
		return nil, errors.New("JWT_PUBLIC_KEY is not a PEM public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("JWT_PUBLIC_KEY: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("JWT_PUBLIC_KEY is not an RSA key")
	}
	return &Verifier{key: key, issuer: issuer, now: now}, nil
}

// Verify returns the caller of a valid token, or an error naming the failed check (for the log).
func (v *Verifier) Verify(token string) (Caller, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Caller{}, errors.New("not three parts")
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := decodePart(parts[0], &header); err != nil {
		return Caller{}, err
	}
	if header.Alg != "RS256" {
		return Caller{}, errors.New("alg is not RS256")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Caller{}, errors.New("signature is not base64url")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(v.key, crypto.SHA256, digest[:], signature) != nil {
		return Caller{}, errors.New("signature does not verify")
	}
	var claims struct {
		Sub  string   `json:"sub"`
		Role string   `json:"role"`
		Name string   `json:"name"`
		Iss  string   `json:"iss"`
		Exp  *float64 `json:"exp"`
	}
	if err := decodePart(parts[1], &claims); err != nil {
		return Caller{}, err
	}
	switch {
	case claims.Iss != v.issuer:
		return Caller{}, errors.New("wrong iss")
	case claims.Exp == nil:
		return Caller{}, errors.New("no exp")
	case v.now().After(time.Unix(int64(*claims.Exp), 0).Add(clockSkew)):
		return Caller{}, errors.New("expired")
	case strings.TrimSpace(claims.Sub) == "":
		return Caller{}, errors.New("no sub")
	case !knownRoles[claims.Role]:
		return Caller{}, errors.New("unknown role")
	}
	return Caller{UserID: claims.Sub, Role: claims.Role, Name: claims.Name}, nil
}

func decodePart(part string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return errors.New("part is not base64url")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return errors.New("part is not a JSON object")
	}
	return nil
}

type callerKey struct{}

// CallerFrom returns the caller the auth middleware left in the request context.
func CallerFrom(ctx context.Context) Caller {
	caller, _ := ctx.Value(callerKey{}).(Caller)
	return caller
}

// WithCaller is for tests of handlers that run without the middleware.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// Auth applies the token rules per route: a user route takes only user tokens of its roles, an
// internal route only service tokens (403 otherwise, D-C27); no valid token is 401. It wraps the
// route's handler, so an unknown route is still 404 and the token is checked before the body.
type Auth struct {
	Verifier *Verifier
}

// User protects a public route for the given roles (any user role when none is given).
func (a Auth) User(roles []string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		if caller.Role == RoleService {
			a.deny(w, r, "service token on a public route")
			return
		}
		if len(roles) > 0 && !contains(roles, caller.Role) {
			a.deny(w, r, "role "+caller.Role+" may not use this route")
			return
		}
		next(w, r.WithContext(WithCaller(r.Context(), caller)))
	}
}

// Service protects an /internal/** route.
func (a Auth) Service(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		if caller.Role != RoleService {
			a.deny(w, r, "user token on an internal route")
			return
		}
		next(w, r.WithContext(WithCaller(r.Context(), caller)))
	}
}

func (a Auth) authenticate(w http.ResponseWriter, r *http.Request) (Caller, bool) {
	header := r.Header.Get("Authorization")
	const bearer = "bearer "
	if len(header) < len(bearer) || !strings.EqualFold(header[:len(bearer)], bearer) {
		unauthorized(w, r, "no bearer token")
		return Caller{}, false
	}
	caller, err := a.Verifier.Verify(strings.TrimSpace(header[len(bearer):]))
	if err != nil {
		unauthorized(w, r, err.Error())
		return Caller{}, false
	}
	return caller, true
}

func unauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	loggerFrom(r.Context()).Warn("Rejected credential: " + reason)
	w.Header().Set("WWW-Authenticate", "Bearer")
	WriteError(w, r, CodeUnauthorized)
}

func (a Auth) deny(w http.ResponseWriter, r *http.Request, reason string) {
	loggerFrom(r.Context()).Warn("Denied: " + reason)
	WriteError(w, r, CodeForbidden)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
