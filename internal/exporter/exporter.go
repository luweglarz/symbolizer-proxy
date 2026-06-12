package exporter

import (
	"context"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
)

type Exporter interface {
	Export(ctx context.Context, req *cprofiles.ExportProfilesServiceRequest) error
}
