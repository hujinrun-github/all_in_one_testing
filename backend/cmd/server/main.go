package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"all_in_one_testing/backend/internal/api"
)

func main() {
	addr := ":8080"
	if value := os.Getenv("APP_ADDR"); value != "" {
		addr = value
	}

	server := &http.Server{
		Addr:    addr,
		Handler: api.NewRouter(),
	}

	slog.Info("starting API server", "addr", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API server stopped", "error", err)
		os.Exit(1)
	}
}
