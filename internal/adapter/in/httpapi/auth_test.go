package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

// The token cases are the same as in the Java services (catalog-api CatalogAuthTest): one
// verifier rule per case, so a mutation of any rule fails exactly one test.
func TestVerifier(t *testing.T) {
	keys := newTestKeys(t)
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	verifier, err := NewVerifier(keys.publicPEMOneLine(t), testIssuer, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	with := func(change func(map[string]any)) map[string]any {
		c := userClaims(now)
		change(c)
		return c
	}
	hs256 := func() string {
		signed := b64(map[string]any{"alg": "HS256"}) + "." + b64(userClaims(now))
		mac := hmac.New(sha256.New, []byte(keys.publicPEMOneLine(t)))
		mac.Write([]byte(signed))
		return signed + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}

	t.Run("a valid token gives its caller", func(t *testing.T) {
		caller, err := verifier.Verify(sign(t, keys.private, rs256(), userClaims(now)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if caller.UserID != "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d" || caller.Role != "BUYER" || caller.Name != "Carlos Pérez" {
			t.Fatalf("caller = %+v", caller)
		}
	})
	t.Run("expired within the 30 s skew is accepted", func(t *testing.T) {
		token := sign(t, keys.private, rs256(), with(func(c map[string]any) { c["exp"] = now.Add(-29 * time.Second).Unix() }))
		if _, err := verifier.Verify(token); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	rejected := map[string]string{
		"expired beyond the skew":   sign(t, keys.private, rs256(), with(func(c map[string]any) { c["exp"] = now.Add(-31 * time.Second).Unix() })),
		"alg none":                  b64(map[string]any{"alg": "none"}) + "." + b64(userClaims(now)) + ".",
		"HS256 with the public key": hs256(),
		"signed with another key":   sign(t, keys.other, rs256(), userClaims(now)),
		"wrong issuer":              sign(t, keys.private, rs256(), with(func(c map[string]any) { c["iss"] = "someone-else" })),
		"no sub":                    sign(t, keys.private, rs256(), with(func(c map[string]any) { delete(c, "sub") })),
		"no exp":                    sign(t, keys.private, rs256(), with(func(c map[string]any) { delete(c, "exp") })),
		"unknown role":              sign(t, keys.private, rs256(), with(func(c map[string]any) { c["role"] = "ROOT" })),
		"not three parts":           "abc.def",
		"part that is not base64":   "%%%.%%%.%%%",
	}
	for name, token := range rejected {
		t.Run(name+" is rejected", func(t *testing.T) {
			if _, err := verifier.Verify(token); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNewVerifierNeedsKeyAndIssuer(t *testing.T) {
	keys := newTestKeys(t)
	if _, err := NewVerifier("", testIssuer, time.Now); err == nil {
		t.Fatal("an empty JWT_PUBLIC_KEY must stop the service")
	}
	if _, err := NewVerifier(keys.publicPEMOneLine(t), " ", time.Now); err == nil {
		t.Fatal("an empty JWT_ISSUER must stop the service")
	}
}
