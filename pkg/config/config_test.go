package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigQuotedEnvironmentAndOverrides(t *testing.T) {
	t.Setenv("CONFIG_FILE", configFile(t, `server:
  http:
    port: "${TEST_CONFIG_PORT:-8080}"
    read_timeout: 2s
auth:
  jwt_public_key: legacy-key
  issuer: example-issuer
  audience: example-audience
spicedb:
  enabled: "${TEST_CONFIG_ENABLED:-false}"
  endpoint: localhost:50051
  token: "${TEST_CONFIG_SECRET}"
  timeout: 1s
redis:
  db: "0"
`))
	t.Setenv("TEST_CONFIG_PORT", "8181")
	t.Setenv("TEST_CONFIG_ENABLED", "true")
	secret := "fake-value\nspicedb:\n  insecure: true\n# : quoted \" content"
	t.Setenv("TEST_CONFIG_SECRET", secret)
	t.Setenv("SPICEDB_TIMEOUT", "3s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "7s")
	t.Setenv("AUTH_JWT_PUBLIC_KEY", "") // An explicit empty override must clear it.
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "0.0.0.0:8181" || cfg.HTTPReadTimeout != 2*time.Second || cfg.HTTPWriteTimeout != 7*time.Second {
		t.Fatal("server settings were not loaded")
	}
	if !cfg.SpiceDB.Enabled || cfg.SpiceDB.Insecure || cfg.SpiceDB.Token != secret || cfg.SpiceDB.Timeout != 3*time.Second {
		t.Fatal("scalar expansion changed configuration structure or lost credentials")
	}
	if cfg.AuthJWTPublicKey != "" || cfg.AuthJWTIssuer != "example-issuer" || cfg.AuthJWTAudience != "example-audience" {
		t.Fatal("auth environment precedence failed")
	}
}

func TestConfigInvalidInputFailsWithoutEchoingValues(t *testing.T) {
	for _, tc := range []struct{ name, contents, envKey, envValue string }{
		{"yaml", "spicedb: [fake-private-value", "", ""},
		{"integer", "server:\n  http:\n    port: fake-private-value", "", ""},
		{"duplicate", "app: {}\napp: {}", "", ""},
		{"bool", "{}", "SPICEDB_ENABLED", "fake-private-value"},
		{"duration", "{}", "SPICEDB_TIMEOUT", "fake-private-value"},
		{"negativeTimeout", "{}", "SPICEDB_TIMEOUT", "-1s"},
		{"redisDB", "{}", "REDIS_DB", "-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CONFIG_FILE", configFile(t, tc.contents))
			if tc.envKey != "" {
				t.Setenv(tc.envKey, tc.envValue)
			}
			_, err := Load()
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "fake-private-value") {
				t.Fatal("configuration error disclosed a value")
			}
		})
	}
}

func TestConfigMissingFileAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := loadFile(path, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	if _, err := Load(); err == nil {
		t.Fatal("explicit missing file was ignored")
	}
	t.Setenv("CONFIG_FILE", configFile(t, "{}"))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SpiceDB.Enabled || cfg.SpiceDB.Insecure || cfg.SpiceDB.Timeout != 5*time.Second || cfg.HTTPAddr != ":8080" || cfg.ShutdownTimeout != 5*time.Second {
		t.Fatal("unexpected defaults")
	}
}

func TestConfigCanonicalAuthKeyWinsOverLegacy(t *testing.T) {
	t.Setenv("CONFIG_FILE", configFile(t, "auth:\n  jwt-public-key: canonical\n  jwt_public_key: legacy\n"))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthJWTPublicKey != "canonical" {
		t.Fatal("canonical key did not win")
	}
}

func TestConfigurationExample(t *testing.T) {
	t.Setenv("CONFIG_FILE", filepath.Join("..", "..", "config", "config.example.yaml"))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDSN != "" || cfg.RedisAddr != "" || cfg.SpiceDB.Enabled {
		t.Fatal("safe example unexpectedly enables an external dependency")
	}
}
