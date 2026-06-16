package exporter

import (
	"bytes"
	"context"
	"flag"
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

func (c *OTLPHTTPConfig) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Endpoint, "exporter.otlphttp.endpoint", "http://localhost:4040", "The endpoint for the OTLP HTTP exporter")
	fs.DurationVar(&c.Timeout, "exporter.otlphttp.timeout", 5*time.Second, "The timeout for the OTLP HTTP exporter")
}

func (e *OTLPHTTPExporter) Export(ctx context.Context, prof *cprofiles.ExportProfilesServiceRequest) error {
	payload, err := proto.Marshal(prof)
	if err != nil {
		return fmt.Errorf("failed to marshal profiles request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/v1development/profiles", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
