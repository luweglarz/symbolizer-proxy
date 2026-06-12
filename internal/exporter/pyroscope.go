package exporter

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	cprofiles "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	"google.golang.org/protobuf/proto"
)

// PyroscopeExporter is an implementation of the Exporter interface for Pyroscope.
type PyroscopeExporter struct {
	client   *http.Client // or a real pyroscope SDK client
	endpoint string
}

func NewPyroscope(endpoint string) *PyroscopeExporter {
	return &PyroscopeExporter{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: endpoint,
	}
}

func (e *PyroscopeExporter) Export(ctx context.Context, req *cprofiles.ExportProfilesServiceRequest) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal profiles request: %w", err)
	}
	resp, err := e.client.Post(e.endpoint+"/v1development/profiles", "application/x-protobuf", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to send profiles request: %w", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
