package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/luweglarz/symbolizer-proxy/internal/exporter"
	"github.com/luweglarz/symbolizer-proxy/internal/server"
	"github.com/luweglarz/symbolizer-proxy/internal/symbolizer"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var expCfg exporter.Config
	var symCfg symbolizer.Config
	var srvCfg server.Config

	expCfg.RegisterFlags(flag.CommandLine)
	symCfg.RegisterFlags(flag.CommandLine)
	srvCfg.RegisterFlags(flag.CommandLine)

	flag.Parse()

	exp, err := exporter.New(expCfg)
	if err != nil {
		logger.Error("failed to create exporter", "err", err)
		os.Exit(1)
	}

	symSource := &symbolizer.FileSource{} // for now top level source
	sym := symbolizer.New(logger, symSource, symCfg)
	if sym == nil {
		logger.Error("failed to create symbolizer")
		os.Exit(1)
	}
	defer sym.Close()

	srv := server.New(sym, exp, srvCfg, logger)

	if err := srv.Start(); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}
