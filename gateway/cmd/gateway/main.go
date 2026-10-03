package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/arnabry11/ev-charging/gateway/internal/auth"
	"github.com/arnabry11/ev-charging/gateway/internal/httpapi"
	"github.com/arnabry11/ev-charging/gateway/internal/ocppserver"
	"github.com/arnabry11/ev-charging/gateway/internal/registry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	store, err := auth.Parse(os.Getenv("CHARGER_AUTH"))
	if err != nil {
		logger.Error("charger auth config", "err", err)
		os.Exit(1)
	}

	reg := registry.New()
	go ocppserver.New(ocppserver.Config{
		Port:               ocppPort(),
		HeartbeatIntervalS: heartbeatInterval(),
		Auth:               store,
		Registry:           reg,
		Logger:             logger,
	}).Start()

	addr := listenAddr()
	logger.Info("gateway http listening", "addr", addr)
	if err := http.ListenAndServe(addr, httpapi.NewRouter(reg)); err != nil {
		logger.Error("http server stopped", "err", err)
		os.Exit(1)
	}
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
