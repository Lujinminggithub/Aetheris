package config

import (
	"encoding/base64"
	"testing"
	"time"
)

func validProjectIdentityKey() string {
	return base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected DATABASE_URL error")
	}
}

func TestLoadUsesModelGatewayTimeoutLongEnoughForLocalInference(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("MODEL_GATEWAY_TIMEOUT", "")
	t.Setenv("AETHERIS_PROJECT_IDENTITY_KEY", validProjectIdentityKey())

	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.ModelGatewayTimeout != 120*time.Second {
		t.Fatalf("model gateway timeout = %s", config.ModelGatewayTimeout)
	}
}

func TestLoadRequiresStrongVersionedProjectIdentityKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AETHERIS_PROJECT_IDENTITY_KEY", base64.StdEncoding.EncodeToString([]byte("too-short")))
	if _, err := Load(); err == nil {
		t.Fatal("expected short project identity key error")
	}

	t.Setenv("AETHERIS_PROJECT_IDENTITY_KEY", validProjectIdentityKey())
	t.Setenv("AETHERIS_PROJECT_IDENTITY_KEY_VERSION", "3")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(config.ProjectIdentityKey) != "0123456789abcdef0123456789abcdef" || config.ProjectIdentityKeyVersion != 3 {
		t.Fatalf("unexpected project identity key configuration")
	}
}
