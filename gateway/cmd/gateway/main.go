package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/arnabry11/ev-charging/gateway/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := listenAddr()
	logger.Info("gateway listening", "addr", addr)
	if err := http.ListenAndServe(addr, httpapi.NewRouter()); err != nil {
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
