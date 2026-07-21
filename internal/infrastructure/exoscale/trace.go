package exoscale

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
)

type metadataRoundTripper struct {
	next   http.RoundTripper
	logger logr.Logger
}

func (t metadataRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	resp, err := t.next.RoundTrip(req)
	duration := time.Since(started).Round(time.Millisecond)
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	fields := []any{
		"method", req.Method,
		"host", req.URL.Host,
		"path", req.URL.EscapedPath(),
		"status", statusCode,
		"duration", duration,
	}
	if err != nil {
		t.logger.Error(err, fmt.Sprintf("HTTP %s %s%s -> REQUEST ERROR (%s)", req.Method, req.URL.Host, req.URL.EscapedPath(), duration), fields...)
		return nil, err
	}

	statusText := http.StatusText(statusCode)
	if statusCode >= http.StatusBadRequest {
		fields = append(fields, "httpError", statusText)
	}
	t.logger.Info(
		fmt.Sprintf("HTTP %s %s%s -> %d %s (%s)", req.Method, req.URL.Host, req.URL.EscapedPath(), statusCode, statusText, duration),
		fields...,
	)
	return resp, nil
}

func metadataHTTPClient(logger logr.Logger) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = nil
	httpClient := retryClient.StandardClient()
	httpClient.Transport = metadataRoundTripper{next: httpClient.Transport, logger: logger}
	return httpClient
}
