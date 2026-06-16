package exporter

import (
	"context"
	"flag"
	"fmt"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
)

type Exporter interface {
	Export(ctx context.Context, req *cprofiles.ExportProfilesServiceRequest) error
}

type Config struct {
	Type     string
	OTLPHTTP OTLPHTTPConfig
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Type, "exporter.type", "otlphttp", "exporter type: otlphttp|noop")
	c.OTLPHTTP.RegisterFlags(fs)
}

func New(cfg Config) (Exporter, error) {
	switch cfg.Type {
	case "otlphttp":
		return NewOTLPHTTP(cfg.OTLPHTTP), nil
	case "noop":
		return NewNoop(), nil
	default:
		return nil, fmt.Errorf("unknown exporter type %q", cfg.Type)
	}
}
