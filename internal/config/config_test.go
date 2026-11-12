package config

import "testing"

func TestLoadRequiresIssuerAndPublicKey(t *testing.T) {
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_PUBLIC_KEY", "")
	t.Setenv("JWT_PUBLIC_KEY_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("the service must not start without JWT_ISSUER and JWT_PUBLIC_KEY")
	}
	t.Setenv("JWT_ISSUER", "market-agri-auth")
	t.Setenv("JWT_PUBLIC_KEY", "pem")
	c, err := Load()
	if err != nil || c.Port != "8080" {
		t.Fatalf("got %+v, %v", c, err)
	}
}
