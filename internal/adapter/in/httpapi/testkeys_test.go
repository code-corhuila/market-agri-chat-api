package httpapi

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"testing"
	"time"
)

const testIssuer = "market-agri-auth"

// testKeys is the RSA pair of a test: tokens are signed here and verified with the public half,
// as auth-api signs and every service verifies (ADR-020).
type testKeys struct {
	private *rsa.PrivateKey
	other   *rsa.PrivateKey
}

func newTestKeys(t *testing.T) testKeys {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	o, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return testKeys{private: k, other: o}
}

// publicPEMOneLine is the public key as JWT_PUBLIC_KEY carries it: PEM on one line with literal \n.
func (k testKeys) publicPEMOneLine(t *testing.T) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&k.private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	out := ""
	for _, b := range string(block) {
		if b == '\n' {
			out += `\n`
		} else {
			out += string(b)
		}
	}
	return out
}

func b64(v any) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// sign builds a token with the given header and claims, signed RS256 with key.
func sign(t *testing.T, key *rsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	signed := b64(header) + "." + b64(claims)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func rs256() map[string]any { return map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test"} }

// claims of a valid BUYER token issued now, expiring in 15 minutes.
func userClaims(now time.Time) map[string]any {
	return map[string]any{
		"sub": "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d", "role": "BUYER", "name": "Carlos Pérez",
		"iss": testIssuer, "iat": now.Unix(), "exp": now.Add(15 * time.Minute).Unix(),
	}
}

func serviceClaims(now time.Time) map[string]any {
	return map[string]any{
		"sub": "service:workflow", "role": "SERVICE",
		"iss": testIssuer, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
}
