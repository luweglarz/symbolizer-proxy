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

type OTLPHTTPConfig struct {
	Endpoint string
	Timeout  time.Duration
}

type OTLPHTTPExporter struct {
	client   *http.Client
	endpoint string
}

func NewOTLPHTTP(cfg OTLPHTTPConfig) *OTLPHTTPExporter {
	return &OTLPHTTPExporter{
		client:   &http.Client{Timeout: cfg.Timeout},
		endpoint: cfg.Endpoint,
	}
}

func (e *OTLPHTTPExporter) Export(ctx context.Context, req *cprofiles.ExportProfilesServiceRequest) error {
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
