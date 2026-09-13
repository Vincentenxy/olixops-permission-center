package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config contains process-level settings. Never log the complete configuration.
type Config struct {
	AppEnv             string
	HTTPAddr           string
	GRPCAddr           string
	HTTPReadTimeout    time.Duration
	HTTPWriteTimeout   time.Duration
	HTTPIdleTimeout    time.Duration
	HTTPTrustedProxies []string
	ShutdownTimeout    time.Duration
	DatabaseDSN        string `json:"-"`
	RedisAddr          string
	RedisPassword      string `json:"-"`
	RedisDB            int
	AuthJWTPublicKey   string
	AuthJWTPrivateKey  string `json:"-"`
	AuthJWTIssuer      string
	AuthJWTAudience    string
	AuthJWTTokenTTL    time.Duration
	SpiceDB            SpiceDB
}

type SpiceDB struct {
	Enabled    bool
	Endpoint   string
	Token      string `json:"-"`
	Insecure   bool
	CAFile     string
	ServerName string
	Timeout    time.Duration
}

// Scalar decoders accept both YAML numbers and quoted environment placeholders.
type scalarInt int

func (v *scalarInt) UnmarshalYAML(node *yaml.Node) error {
	n, err := strconv.Atoi(node.Value)
	if err != nil {
		return errors.New("expected an integer")
	}
	*v = scalarInt(n)
	return nil
}

type scalarBool bool

func (v *scalarBool) UnmarshalYAML(node *yaml.Node) error {
	b, err := strconv.ParseBool(node.Value)
	if err != nil {
		return errors.New("expected a boolean")
	}
	*v = scalarBool(b)
	return nil
}

type fileConfig struct {
	App    struct{ Environment string } `yaml:"app"`
	Server struct {
		HTTP struct {
			Host           string
			Port           scalarInt
			ReadTimeout    string   `yaml:"read_timeout"`
			WriteTimeout   string   `yaml:"write_timeout"`
			IdleTimeout    string   `yaml:"idle_timeout"`
			TrustedProxies []string `yaml:"trusted_proxies"`
		} `yaml:"http"`
		GRPC struct {
			Host string
			Port scalarInt
		} `yaml:"grpc"`
		ShutdownTimeout string `yaml:"shutdown_timeout"`
	} `yaml:"server"`
	Auth struct {
		PublicKey       string `yaml:"jwt-public-key"`
		PrivateKey      string `yaml:"jwt-private-key"`
		LegacyPublicKey string `yaml:"jwt_public_key"`
		Issuer          string `yaml:"issuer"`
		Audience        string `yaml:"audience"`
		TokenTTL        string `yaml:"token-ttl"`
	} `yaml:"auth"`
	Database struct {
		Host     string
		Port     scalarInt
		User     string
		Password string
		Name     string
		Schema   string
		SSLMode  string `yaml:"ssl_mode"`
	} `yaml:"database"`
	Redis struct {
		Enabled  *scalarBool
		Addrs    []string
		Password string
		DB       scalarInt
	} `yaml:"redis"`
	SpiceDB struct {
		Enabled    scalarBool
		Endpoint   string
		Token      string
		Insecure   scalarBool
		CAFile     string `yaml:"ca-file"`
		ServerName string `yaml:"server-name"`
		Timeout    string
	} `yaml:"spicedb"`
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv: "development", HTTPAddr: ":8080", GRPCAddr: ":9090",
		HTTPReadTimeout: 10 * time.Second, HTTPWriteTimeout: 30 * time.Second,
		HTTPIdleTimeout: 60 * time.Second, ShutdownTimeout: 5 * time.Second,
		AuthJWTTokenTTL: 8 * time.Hour,
		SpiceDB:         SpiceDB{Timeout: 5 * time.Second},
	}
	path, explicit := os.LookupEnv("CONFIG_FILE")
	if !explicit || path == "" {
		path, explicit = "config/config.yaml", false
	}
	source, err := loadFile(path, explicit)
	if err != nil {
		return Config{}, err
	}
	if source.App.Environment != "" {
		cfg.AppEnv = source.App.Environment
	}
	if source.Server.HTTP.Port != 0 {
		cfg.HTTPAddr = listenAddr(source.Server.HTTP.Host, int(source.Server.HTTP.Port))
	}
	if source.Server.GRPC.Port != 0 {
		cfg.GRPCAddr = listenAddr(source.Server.GRPC.Host, int(source.Server.GRPC.Port))
	}
	cfg.AuthJWTPublicKey = source.Auth.PublicKey
	cfg.HTTPTrustedProxies = append([]string(nil), source.Server.HTTP.TrustedProxies...)
	if raw, exists := os.LookupEnv("HTTP_TRUSTED_PROXIES"); exists {
		cfg.HTTPTrustedProxies = nil
		if strings.TrimSpace(raw) != "" {
			cfg.HTTPTrustedProxies = strings.Split(raw, ",")
		}
	}
	for i, value := range cfg.HTTPTrustedProxies {
		value = strings.TrimSpace(value)
		if net.ParseIP(value) == nil {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return Config{}, errors.New("HTTP_TRUSTED_PROXIES must contain IP addresses or CIDR ranges")
			}
		}
		cfg.HTTPTrustedProxies[i] = value
	}
	if cfg.AuthJWTPublicKey == "" {
		cfg.AuthJWTPublicKey = source.Auth.LegacyPublicKey
	}
	cfg.AuthJWTIssuer, cfg.AuthJWTAudience = source.Auth.Issuer, source.Auth.Audience
	cfg.AuthJWTPrivateKey = source.Auth.PrivateKey
	if source.Database.Host != "" && source.Database.Name != "" {
		port := int(source.Database.Port)
		if port == 0 {
			port = 5432
		}
		u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(source.Database.Host, strconv.Itoa(port)), Path: "/" + source.Database.Name, User: url.UserPassword(source.Database.User, source.Database.Password)}
		q := u.Query()
		if source.Database.Schema != "" {
			q.Set("search_path", source.Database.Schema)
		}
		if source.Database.SSLMode != "" {
			q.Set("sslmode", source.Database.SSLMode)
		}
		u.RawQuery = q.Encode()
		cfg.DatabaseDSN = u.String()
	}
	if len(source.Redis.Addrs) > 1 {
		return Config{}, errors.New("redis supports one address in this initialization")
	}
	if len(source.Redis.Addrs) == 1 {
		cfg.RedisAddr = source.Redis.Addrs[0]
	}
	if source.Redis.Enabled != nil && !bool(*source.Redis.Enabled) {
		cfg.RedisAddr = ""
	}
	cfg.RedisPassword, cfg.RedisDB = source.Redis.Password, int(source.Redis.DB)
	cfg.SpiceDB.Enabled, cfg.SpiceDB.Insecure = bool(source.SpiceDB.Enabled), bool(source.SpiceDB.Insecure)
	cfg.SpiceDB.Endpoint, cfg.SpiceDB.Token = source.SpiceDB.Endpoint, source.SpiceDB.Token
	cfg.SpiceDB.CAFile, cfg.SpiceDB.ServerName = source.SpiceDB.CAFile, source.SpiceDB.ServerName
	for key, target := range map[string]*string{
		"APP_ENV": &cfg.AppEnv, "HTTP_ADDR": &cfg.HTTPAddr, "GRPC_ADDR": &cfg.GRPCAddr,
		"DATABASE_DSN": &cfg.DatabaseDSN, "REDIS_ADDR": &cfg.RedisAddr, "REDIS_PASSWORD": &cfg.RedisPassword,
		"AUTH_JWT_PUBLIC_KEY": &cfg.AuthJWTPublicKey, "AUTH_JWT_ISSUER": &cfg.AuthJWTIssuer, "AUTH_JWT_AUDIENCE": &cfg.AuthJWTAudience,
		"AUTH_JWT_PRIVATE_KEY": &cfg.AuthJWTPrivateKey,
		"SPICEDB_ENDPOINT":     &cfg.SpiceDB.Endpoint, "SPICEDB_TOKEN": &cfg.SpiceDB.Token,
		"SPICEDB_CA_FILE": &cfg.SpiceDB.CAFile, "SPICEDB_SERVER_NAME": &cfg.SpiceDB.ServerName,
	} {
		if value, exists := os.LookupEnv(key); exists {
			*target = value
		}
	}
	for key, target := range map[string]*bool{"SPICEDB_ENABLED": &cfg.SpiceDB.Enabled, "SPICEDB_INSECURE": &cfg.SpiceDB.Insecure} {
		if value, exists := os.LookupEnv(key); exists {
			parsed, parseErr := strconv.ParseBool(value)
			if parseErr != nil {
				return Config{}, fmt.Errorf("%s must be a boolean", key)
			}
			*target = parsed
		}
	}
	if value, exists := os.LookupEnv("REDIS_DB"); exists {
		cfg.RedisDB, err = strconv.Atoi(value)
		if err != nil {
			return Config{}, errors.New("REDIS_DB must be an integer")
		}
	}
	if cfg.RedisDB < 0 {
		return Config{}, errors.New("redis database index must not be negative")
	}
	for _, duration := range []struct {
		key, file string
		target    *time.Duration
	}{
		{"HTTP_READ_TIMEOUT", source.Server.HTTP.ReadTimeout, &cfg.HTTPReadTimeout},
		{"HTTP_WRITE_TIMEOUT", source.Server.HTTP.WriteTimeout, &cfg.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", source.Server.HTTP.IdleTimeout, &cfg.HTTPIdleTimeout},
		{"SHUTDOWN_TIMEOUT", source.Server.ShutdownTimeout, &cfg.ShutdownTimeout},
		{"AUTH_JWT_TOKEN_TTL", source.Auth.TokenTTL, &cfg.AuthJWTTokenTTL},
		{"SPICEDB_TIMEOUT", source.SpiceDB.Timeout, &cfg.SpiceDB.Timeout},
	} {
		value := duration.file
		if override, exists := os.LookupEnv(duration.key); exists {
			value = override
		}
		if value == "" {
			continue
		}
		parsed, parseErr := time.ParseDuration(value)
		if parseErr != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", duration.key)
		}
		*duration.target = parsed
	}
	if cfg.AuthJWTTokenTTL < time.Second || cfg.AuthJWTTokenTTL > 24*time.Hour {
		return Config{}, errors.New("AUTH_JWT_TOKEN_TTL must be between 1s and 24h")
	}
	return cfg, nil
}

func loadFile(path string, required bool) (fileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return fileConfig{}, nil
		}
		return fileConfig{}, errors.New("cannot read configuration file")
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return fileConfig{}, errors.New("invalid configuration YAML")
	}
	expandNode(&document)
	var source fileConfig
	if err := document.Decode(&source); err != nil {
		return fileConfig{}, errors.New("invalid configuration field type or value")
	}
	return source, nil
}

var environmentPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// Expand scalar values after parsing so credentials can never inject YAML keys.
func expandNode(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		node.Value = environmentPattern.ReplaceAllStringFunc(node.Value, func(expression string) string {
			parts := environmentPattern.FindStringSubmatch(expression)
			if value, exists := os.LookupEnv(parts[1]); exists && value != "" {
				return value
			}
			if len(parts) > 3 {
				return parts[3]
			}
			return ""
		})
	}
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			continue
		}
		expandNode(child)
	}
}

func listenAddr(host string, port int) string {
	if strings.TrimSpace(host) == "" {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
