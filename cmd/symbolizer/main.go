package main

import (
	"log/slog"
	"os"

	"github.com/luweglarz/symbolizer-proxy/internal/exporter"
	"github.com/luweglarz/symbolizer-proxy/internal/server"
	"github.com/luweglarz/symbolizer-proxy/internal/symbolizer"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sym := symbolizer.New()
	pyroscopeURL := os.Getenv("PYROSCOPE_URL")
	if pyroscopeURL == "" {
		pyroscopeURL = "http://localhost:4040"
	}
	exporter := exporter.NewPyroscope(pyroscopeURL)
	serv := server.New(sym, exporter, ":8080", logger)

	if err := serv.Start(); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}
