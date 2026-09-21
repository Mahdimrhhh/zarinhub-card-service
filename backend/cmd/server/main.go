package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"card-service/backend/internal/client"
	"card-service/backend/internal/handler"
	"card-service/backend/internal/logging"
	"card-service/backend/internal/repository"
	"card-service/backend/internal/service"
)

type config struct {
	zarinHubToken    string
	zarinHubEndpoint string
	zarinHubTimeout  time.Duration
	databasePath     string
	serverAddress    string
	logLevel         string
	corsOrigin       string
}

func main() {
	cfg := loadConfig()
	logger, err := logging.NewLogger(cfg.logLevel)
	if err != nil {
		slog.Default().Error("invalid log level", "error", err)
		os.Exit(1)
	}

	databaseDirectory := filepath.Dir(cfg.databasePath)
	if databaseDirectory != "." {
		if err := os.MkdirAll(databaseDirectory, 0o750); err != nil {
			logger.Error("database directory creation failed", "error_category", "configuration")
			os.Exit(1)
		}
	}

	db, err := repository.Open(cfg.databasePath)
	if err != nil {
		logger.Error("database open failed", "error_category", "persistence")
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	if err := repository.Migrate(ctx, db); err != nil {
		logger.Error("database migration failed", "error_category", "persistence")
		os.Exit(1)
	}

	zarinHub, err := client.NewZarinHubClient(cfg.zarinHubEndpoint, cfg.zarinHubToken, cfg.zarinHubTimeout)
	if err != nil {
		logger.Error("zarin-hub client configuration failed", "error_category", "configuration")
		os.Exit(1)
	}

	repo := repository.NewSQLiteRepository(db)
	appService := service.NewService(zarinHub, repo, logger)
	httpHandler := handler.NewHandler(appService, logger, cfg.corsOrigin)

	server := &http.Server{
		Addr:              cfg.serverAddress,
		Handler:           httpHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", cfg.serverAddress)
		serverErrors <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error_category", "internal")
			os.Exit(1)
		}
	case <-stop:
		logger.Info("server shutdown started")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error_category", "internal")
			os.Exit(1)
		}
	}
}

func loadConfig() config {
	return config{
		zarinHubToken:    getEnv("ZARINHUB_ACCESS_TOKEN", ""),
		zarinHubEndpoint: getEnv("ZARINHUB_API_URL", client.DefaultEndpoint),
		zarinHubTimeout:  getDuration("ZARINHUB_TIMEOUT", 5*time.Second),
		databasePath:     getEnv("DATABASE_PATH", "./data/card-service.db"),
		serverAddress:    getEnv("SERVER_ADDR", ":8080"),
		logLevel:         getEnv("LOG_LEVEL", "info"),
		corsOrigin:       getEnv("CORS_ORIGIN", "http://localhost:3000"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		slog.Default().Error("invalid duration environment variable", "key", key)
		os.Exit(1)
	}
	return parsed
}
