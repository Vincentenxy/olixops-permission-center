package config

import (
	"encoding/json"
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
		{"invalidTokenTTL", "{}", "AUTH_JWT_TOKEN_TTL", "fake-private-value"},
		{"negativeTokenTTL", "{}", "AUTH_JWT_TOKEN_TTL", "-1s"},
		{"shortTokenTTL", "{}", "AUTH_JWT_TOKEN_TTL", "1ns"},
		{"longTokenTTL", "{}", "AUTH_JWT_TOKEN_TTL", "25h"},
		{"redisDB", "{}", "REDIS_DB", "-1"},
		{"trustedProxy", "{}", "HTTP_TRUSTED_PROXIES", "fake-private-value"},
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

func TestTrustedProxyConfiguration(t *testing.T) {
	t.Setenv("CONFIG_FILE", configFile(t, "server:\n  http:\n    trusted_proxies: [127.0.0.1, '192.0.2.0/24']\n"))
	// Explicit empty override disables all file-configured proxies.
	t.Setenv("HTTP_TRUSTED_PROXIES", "")
	cfg, err := Load()
	if err != nil || len(cfg.HTTPTrustedProxies) != 0 {
		t.Fatal("empty override did not disable proxy trust")
	}
	t.Setenv("HTTP_TRUSTED_PROXIES", " 192.0.2.0/24, ::1 ")
	cfg, err = Load()
	if err != nil || len(cfg.HTTPTrustedProxies) != 2 || cfg.HTTPTrustedProxies[0] != "192.0.2.0/24" || cfg.HTTPTrustedProxies[1] != "::1" {
		t.Fatal("trusted proxy override was not normalized")
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
	if cfg.AuthJWTTokenTTL != 8*time.Hour {
		t.Fatal("unexpected authentication token lifetime")
	}
}

func TestLocalAuthConfigurationAndSecretRedaction(t *testing.T) {
	t.Setenv("CONFIG_FILE", configFile(t, "auth:\n  jwt-private-key: file-private-placeholder\n  token-ttl: 2h\n"))
	cfg, err := Load()
	if err != nil || cfg.AuthJWTPrivateKey != "file-private-placeholder" || cfg.AuthJWTTokenTTL != 2*time.Hour {
		t.Fatal("local auth file settings not loaded")
	}
	t.Setenv("AUTH_JWT_PRIVATE_KEY", "environment-private-placeholder")
	t.Setenv("AUTH_JWT_TOKEN_TTL", "3h")
	cfg, err = Load()
	if err != nil || cfg.AuthJWTPrivateKey != "environment-private-placeholder" || cfg.AuthJWTTokenTTL != 3*time.Hour {
		t.Fatal("local auth environment overrides not loaded")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(encoded), "environment-private-placeholder") {
		t.Fatal("serialized configuration disclosed private key")
	}
	t.Setenv("AUTH_JWT_PRIVATE_KEY", "")
	cfg, err = Load()
	if err != nil || cfg.AuthJWTPrivateKey != "" {
		t.Fatal("explicit empty override did not clear private key")
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
