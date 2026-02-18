package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type ServerCfg struct {
	Host               string   `yaml:"host"`
	Port               string   `yaml:"port"`
	CORSOrigins        []string `yaml:"cors_origins"`
	RateLimitPerMinute int      `yaml:"rate_limit_per_minute"`
}

type PostgresCfg struct {
	DSN string `yaml:"dsn"`
}

type RedisCfg struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type SecurityCfg struct {
	APIKeyPepper            string `yaml:"api_key_pepper"`
	APIKeyCacheTTLSeconds   int    `yaml:"api_key_cache_ttl_seconds"`
	ProviderKeyEncryptionKey string `yaml:"provider_key_encryption_key"`
}

type Config struct {
	Environment string      `yaml:"environment"`
	Server      ServerCfg   `yaml:"server"`
	Postgres    PostgresCfg `yaml:"postgres"`
	Redis       RedisCfg    `yaml:"redis"`
	Security    SecurityCfg `yaml:"security"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyEnvOverrides(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("API_KEY_PEPPER"); v != "" {
		cfg.Security.APIKeyPepper = v
	}
	if v := os.Getenv("POSTGRES_DB_URL"); v != "" {
		cfg.Postgres.DSN = v
	}
	// CORS_ORIGINS accepts a comma-separated list of allowed origins,
	// e.g. "https://app.vercel.app,https://burnrate.ai"
	// Setting it to "*" allows all origins (useful during development).
	if v := os.Getenv("PROVIDER_KEY_ENCRYPTION_KEY"); v != "" {
		cfg.Security.ProviderKeyEncryptionKey = v
	}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		if v == "*" {
			cfg.Server.CORSOrigins = []string{"*"}
		} else {
			origins := strings.Split(v, ",")
			for i, o := range origins {
				origins[i] = strings.TrimSpace(o)
			}
			cfg.Server.CORSOrigins = origins
		}
	}
}
