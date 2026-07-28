package exoscale

import (
	"context"
	"fmt"
	"net/http"
	"time"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/go-logr/logr"
)

const operationWaitTimeout = 10 * time.Minute

func New(apiKey, apiSecret string, zone egoscale.ZoneName) (*egoscale.Client, error) {
	return newClient(apiKey, apiSecret, zone, nil)
}

// NewCloud returns the cluster-scoped cloud API backed by the current adapter.
func NewCloud(apiKey, apiSecret string, zone egoscale.ZoneName) (*Adapter, error) {
	client, err := New(apiKey, apiSecret, zone)
	if err != nil {
		return nil, err
	}
	return NewAdapter(client), nil
}

func NewLogging(apiKey, apiSecret string, zone egoscale.ZoneName, logger logr.Logger) (*egoscale.Client, error) {
	return newClient(apiKey, apiSecret, zone, metadataHTTPClient(logger))
}

func newClient(apiKey, apiSecret string, zone egoscale.ZoneName, httpClient *http.Client) (*egoscale.Client, error) {
	opts := []egoscale.ClientOpt{egoscale.ClientOptWithWaitTimeout(operationWaitTimeout)}
	if httpClient != nil {
		opts = append(opts, egoscale.ClientOptWithHTTPClient(httpClient))
	}

	exoClient, err := egoscale.NewClient(
		credentials.NewStaticCredentials(apiKey, apiSecret),
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create exoscale client: %w", err)
	}

	endpoint, err := exoClient.GetZoneAPIEndpoint(context.Background(), zone)
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoint for zone %q: %w", string(zone), err)
	}

	return exoClient.WithEndpoint(endpoint), nil
}
