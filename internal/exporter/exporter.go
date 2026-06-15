package exporter

import (
	"context"
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
