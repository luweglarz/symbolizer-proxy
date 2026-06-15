package exporter

import (
	"context"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
)

type NoopExporter struct{}

func NewNoop() *NoopExporter {
	return &NoopExporter{}
}

func (e *NoopExporter) Export(ctx context.Context, req *cprofiles.ExportProfilesServiceRequest) error {
	return nil
}
