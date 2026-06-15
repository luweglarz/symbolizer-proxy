package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/luweglarz/symbolizer-proxy/internal/exporter"
	"github.com/luweglarz/symbolizer-proxy/internal/server"
	"github.com/luweglarz/symbolizer-proxy/internal/symbolizer"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var expCfg exporter.Config
	var srvCfg server.Config

	flag.StringVar(&expCfg.Type, "exporter.type", "otlphttp", "exporter type: otlphttp|noop")
	flag.DurationVar(&expCfg.OTLPHTTP.Timeout, "exporter.otlphttp.timeout", 5*time.Second, "OTLP HTTP exporter timeout")
	flag.StringVar(&expCfg.OTLPHTTP.Endpoint, "exporter.otlphttp.endpoint", "http://localhost:4040", "OTLP HTTP exporter endpoint")
	flag.StringVar(&srvCfg.Addr, "server.addr", ":8080", "listen address")
	flag.Parse()

	exp, err := exporter.New(expCfg)
	if err != nil {
		logger.Error("failed to create exporter", "err", err)
		os.Exit(1)
	}

	sym := symbolizer.New()

	srv := server.New(sym, exp, srvCfg, logger)

	if err := srv.Start(); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}
