package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config captures runtime configuration for the Qdrant-backed registry.
type Config struct {
	ServiceName             string        `envconfig:"SERVICE_NAME" default:"qdrant-registry"`
	HTTPAddress             string        `envconfig:"HTTP_ADDRESS" default:":8080"`
	GracefulShutdownTimeout time.Duration `envconfig:"GRACEFUL_SHUTDOWN_TIMEOUT" default:"20s"`

	QdrantHost       string `envconfig:"HOST" default:"localhost"`
	QdrantPort       int    `envconfig:"PORT" default:"6334"`
	QdrantAPIKey     string `envconfig:"API_KEY"`
	QdrantCollection string `envconfig:"COLLECTION" default:"mcp_tools"`
	EmbeddingDim     int    `envconfig:"EMBEDDING_DIM" required:"true"`

	DefaultVisibility string `envconfig:"DEFAULT_VISIBILITY" default:"public"`

	MetricsEnabled bool   `envconfig:"METRICS_ENABLED" default:"false"`
	LogLevel       string `envconfig:"LOG_LEVEL" default:"info"`

	VectorizeURL    string `envconfig:"VECTORIZE_URL"`
	VectorizeAPIKey string `envconfig:"VECTORIZE_API_KEY"`
	VectorizeModel  string `envconfig:"VECTORIZE_MODEL" default:"nomic-embed-text"`
}

// Load reads env vars prefixed with MCP_QDRANT_.
func Load() (Config, error) {
	var cfg Config
	if err := envconfig.Process("MCP_QDRANT", &cfg); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	if cfg.EmbeddingDim <= 0 {
		return Config{}, fmt.Errorf("MCP_QDRANT_EMBEDDING_DIM must be > 0")
	}
	if cfg.QdrantHost == "" {
		return Config{}, fmt.Errorf("MCP_QDRANT_HOST is required")
	}
	if cfg.QdrantPort <= 0 {
		return Config{}, fmt.Errorf("MCP_QDRANT_PORT must be > 0")
	}
	return cfg, nil
}
