package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config captures runtime configuration sourced from the environment.
type Config struct {
	ServiceName             string        `envconfig:"SERVICE_NAME" default:"mcp-registry"`
	HTTPAddress             string        `envconfig:"HTTP_ADDRESS" default:":8080"`
	GracefulShutdownTimeout time.Duration `envconfig:"GRACEFUL_SHUTDOWN_TIMEOUT" default:"20s"`

	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	RedisEnabled bool          `envconfig:"REDIS_ENABLED" default:"false"`
	RedisAddr    string        `envconfig:"REDIS_ADDR" default:"127.0.0.1:6379"`
	RedisDB      int           `envconfig:"REDIS_DB" default:"0"`
	RedisTimeout time.Duration `envconfig:"REDIS_TIMEOUT" default:"5s"`

	MetricsEnabled bool   `envconfig:"METRICS_ENABLED" default:"true"`
	LogLevel       string `envconfig:"LOG_LEVEL" default:"info"`
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	var cfg Config
	if err := envconfig.Process("MCP_REGISTRY", &cfg); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	if cfg.RedisEnabled && cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("redis enabled but MCP_REGISTRY_REDIS_ADDR not provided")
	}

	return cfg, nil
}
