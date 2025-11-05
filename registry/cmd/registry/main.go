package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	httpapi "github.com/ashrafalaodat/registry/internal/api/http"
	"github.com/ashrafalaodat/registry/internal/config"
	"github.com/ashrafalaodat/registry/internal/persistence/postgres"
	"github.com/ashrafalaodat/registry/internal/ranking"
	"github.com/ashrafalaodat/registry/internal/service"
	"github.com/ashrafalaodat/registry/internal/vectorizer"
	"github.com/ashrafalaodat/registry/pkg/logging"
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

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to create postgres pool", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}

	repo := postgres.NewRepository(pool)
	var vec service.Vectorizer
	if cfg.VectorizeURL != "" {
		vec = vectorizer.New(cfg.VectorizeURL, cfg.VectorizeAPIKey, cfg.VectorizeModel)
	} else {
		logger.Warn("vectorizer URL not configured; embeddings will be empty")
	}

	var reranker service.Reranker
	if cfg.RerankURL != "" {
		reranker = ranking.NewClient(cfg.RerankURL, cfg.RerankAPIKey, cfg.RerankModel, cfg.RerankTopN)
		if reranker == nil {
			logger.Warn("rerank URL configured but client could not be created")
		}
	} else {
		logger.Info("reranker URL not configured; falling back to vector distance ordering")
	}

	svc := service.New(repo, vec, reranker)
	handler := httpapi.NewHandler(svc, logger)
	router := httpapi.NewRouter(handler, cfg.MetricsEnabled)

	server := &http.Server{
		Addr:         cfg.HTTPAddress,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		logger.Info("starting registry server", zap.String("addr", cfg.HTTPAddress))
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
