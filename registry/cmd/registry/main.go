package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/ashrafalaodat/smcp/registry/internal/api/http"
	"github.com/ashrafalaodat/smcp/registry/internal/config"
	"github.com/ashrafalaodat/smcp/registry/internal/persistence/qdrant"
	"github.com/ashrafalaodat/smcp/registry/internal/service"
	"github.com/ashrafalaodat/smcp/registry/internal/vectorizer"
	"github.com/ashrafalaodat/smcp/registry/pkg/logging"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo, err := qdrantrepo.New(ctx, qdrantrepo.Config{
		Host:       cfg.QdrantHost,
		Port:       cfg.QdrantPort,
		APIKey:     cfg.QdrantAPIKey,
		Collection: cfg.QdrantCollection,
		Dimension:  cfg.EmbeddingDim,
	})
	if err != nil {
		logger.Fatal("failed to initialize qdrant repository", zap.Error(err))
	}
	defer repo.Close()

	if cfg.VectorizeURL == "" {
		logger.Fatal("MCP_QDRANT_VECTORIZE_URL must be set to generate embeddings")
	}
	vec := vectorizer.New(cfg.VectorizeURL, cfg.VectorizeAPIKey, cfg.VectorizeModel)

	svc := service.New(repo, vec, cfg.EmbeddingDim, cfg.DefaultVisibility)
	handler := httpapi.NewHandler(svc, logger)
	router := httpapi.NewRouter(handler)

	server := &http.Server{
		Addr:         cfg.HTTPAddress,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logger.Info("starting qdrant registry", zap.String("addr", cfg.HTTPAddress))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("http server failed", zap.Error(err))
		}
	}()

	waitForShutdown(ctx, logger, server, cfg.GracefulShutdownTimeout)
}

func waitForShutdown(ctx context.Context, logger *zap.Logger, server *http.Server, timeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down gracefully")
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
}
