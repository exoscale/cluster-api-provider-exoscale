package exoscale

import (
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
)

const apiMetadataLogLevel = 4

type metadataRoundTripper struct {
	next   http.RoundTripper
	logger logr.Logger
}

type requestError struct{ cause error }

func (requestError) Error() string   { return "request failed" }
func (e requestError) Unwrap() error { return e.cause }

func (t metadataRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	resp, err := t.next.RoundTrip(req)
	duration := time.Since(started).Round(time.Millisecond)
	statusCode := 0
	statusText := ""
	if resp != nil {
		statusCode = resp.StatusCode
		statusText = http.StatusText(statusCode)
	}
	var safeErr requestError
	if err != nil {
		safeErr = requestError{cause: err}
		statusText = safeErr.Error()
	}
	fields := []any{
		"method", req.Method,
		"host", req.URL.Host,
		"path", req.URL.EscapedPath(),
		"status", statusCode,
		"duration", duration,
		"statusText", statusText,
	}
	logger := t.logger.V(apiMetadataLogLevel)
	if logger.Enabled() {
		if err != nil {
			logger.Error(safeErr, "Exoscale API request", fields...)
		} else {
			logger.Info("Exoscale API request", fields...)
		}
	}
	if err != nil {
		// http.Client includes req.URL in its outer error, so retain only the safe host and path.
		req.URL.User = nil
		req.URL.RawQuery = ""
		req.URL.ForceQuery = false
		req.URL.Fragment = ""
		req.URL.RawFragment = ""
		return nil, safeErr
	}
	return resp, nil
}

func metadataHTTPClient(logger logr.Logger) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = nil
	httpClient := retryClient.StandardClient()
	httpClient.Transport = metadataRoundTripper{next: httpClient.Transport, logger: logger}
	return httpClient
}
