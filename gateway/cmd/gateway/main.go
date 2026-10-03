package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/arnabry11/ev-charging/gateway/internal/auth"
	"github.com/arnabry11/ev-charging/gateway/internal/httpapi"
	"github.com/arnabry11/ev-charging/gateway/internal/ocppserver"
	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/arnabry11/ev-charging/gateway/internal/session"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	authStore, err := auth.Parse(os.Getenv("CHARGER_AUTH"))
	if err != nil {
		logger.Error("charger auth config", "err", err)
		os.Exit(1)
	}

	reg := registry.New()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, requiredEnv("DATABASE_URL"))
	if err != nil {
		logger.Error("database config", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("database unavailable", "err", err)
		os.Exit(1)
	}

	ocpp := ocppserver.New(ocppserver.Config{
		Port:               ocppPort(),
		HeartbeatIntervalS: heartbeatInterval(),
		Auth:               authStore,
		Registry:           reg,
		Logger:             logger,
	})
	sessions := session.New(store.New(pool), reg, ocpp)
	ocpp.SetSessionHandler(sessions)
	go ocpp.Start()

	addr := listenAddr()
	logger.Info("gateway http listening", "addr", addr)
	router := httpapi.NewRouter(httpapi.Dependencies{
		Chargers:      reg,
		Sessions:      sessions,
		SigningSecret: requiredEnv("PLATFORM_SIGNING_SECRET"),
	})
	if err := http.ListenAndServe(addr, router); err != nil {
		logger.Error("http server stopped", "err", err)
		os.Exit(1)
	}
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		slog.Error("required environment variable is missing", "key", key)
		os.Exit(1)
	}
	return value
}

func listenAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

func ocppPort() int {
	raw := os.Getenv("OCPP_PORT")
	if raw == "" {
		return 9000
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 {
		return 9000
	}
	return port
}

func heartbeatInterval() int {
	raw := os.Getenv("HEARTBEAT_INTERVAL_S")
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	return n
}
