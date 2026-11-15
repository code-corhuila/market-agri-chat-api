package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresEveryConnection(t *testing.T) {
	for _, v := range []string{"JWT_ISSUER", "JWT_PUBLIC_KEY", "JWT_PUBLIC_KEY_FILE", "MONGO_URL", "CATALOG_API_URL", "SERVICE_TOKEN"} {
		t.Setenv(v, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("the service must not start without its variables")
	}
	t.Setenv("JWT_ISSUER", "market-agri-auth")
	t.Setenv("JWT_PUBLIC_KEY", "pem")
	if _, err := Load(); err == nil {
		t.Fatal("MONGO_URL, CATALOG_API_URL and SERVICE_TOKEN are required too")
	}
	t.Setenv("MONGO_URL", "mongodb://chat_app:x@mongo:27017/chat?authSource=chat&replicaSet=rs0")
	t.Setenv("CATALOG_API_URL", "http://catalog-api:8080")
	t.Setenv("SERVICE_TOKEN", "token")
	c, err := Load()
	if err != nil || c.Port != "8080" || c.MongoDatabase != "chat" || c.HTTPAttempts != 3 {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestEachConnectionIsRequired(t *testing.T) {
	full := map[string]string{
		"JWT_ISSUER": "market-agri-auth", "JWT_PUBLIC_KEY": "pem", "JWT_PUBLIC_KEY_FILE": "",
		"MONGO_URL": "mongodb://mongo", "CATALOG_API_URL": "http://catalog-api:8080", "SERVICE_TOKEN": "token",
	}
	for _, missing := range []string{"MONGO_URL", "CATALOG_API_URL", "SERVICE_TOKEN"} {
		for name, value := range full {
			t.Setenv(name, value)
		}
		t.Setenv(missing, " ")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("without %s: %v", missing, err)
		}
	}
}
