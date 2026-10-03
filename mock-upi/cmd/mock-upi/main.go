package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/arnabry11/ev-charging/mock-upi/internal/upi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	secret := getenv("WEBHOOK_SECRET", "dev-upi-secret")
	server := upi.NewServer(upi.NewStore(secret), os.Getenv("WEBHOOK_URL"), secret, logger)
	addr := ":" + getenv("PORT", "8082")
	logger.Info("mock upi listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		logger.Error("mock upi stopped", "err", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
