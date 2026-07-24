package exoscale

import (
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	apiMetadataLogLevel = 4
	apiWireLogLevel     = 9
	apiBodyLogLevel     = 10
)

type metadataRoundTripper struct {
	next   http.RoundTripper
	logger logr.Logger
}

type unsafeTraceRoundTripper struct {
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

func (t unsafeTraceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	logger := t.logger.V(apiWireLogLevel)
	withBody := t.logger.V(apiBodyLogLevel).Enabled()

	request := req.Clone(req.Context())
	dumpBody := withBody
	if withBody && req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			dumpBody = false
			logger.Info("Unable to dump non-replayable Exoscale API request body")
		} else {
			body, err := req.GetBody()
			if err != nil {
				dumpBody = false
				logger.Info("Unable to dump Exoscale API request body", "error", err)
			} else {
				request.Body = body
			}
		}
	}
	if dump, err := httputil.DumpRequestOut(request, dumpBody); err != nil {
		logger.Info("Unable to dump Exoscale API request", "error", err)
	} else {
		logger.Info("Unsafe Exoscale API wire request", "dump", string(dump))
	}

	resp, err := t.next.RoundTrip(req)
	if resp != nil {
		if dump, dumpErr := httputil.DumpResponse(resp, withBody); dumpErr != nil {
			logger.Info("Unable to dump Exoscale API response", "error", dumpErr)
		} else {
			logger.Info("Unsafe Exoscale API wire response", "dump", string(dump))
		}
	}
	return resp, err
}

func loggingRoundTripper(next http.RoundTripper, logger logr.Logger, unsafeAPITrace bool) http.RoundTripper {
	transport := http.RoundTripper(metadataRoundTripper{next: next, logger: logger})
	if unsafeAPITrace && logger.V(apiWireLogLevel).Enabled() {
		transport = unsafeTraceRoundTripper{next: transport, logger: logger}
	}
	return transport
}

func loggingHTTPClient(logger logr.Logger, unsafeAPITrace bool) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = nil
	httpClient := retryClient.StandardClient()
	httpClient.Transport = loggingRoundTripper(httpClient.Transport, logger, unsafeAPITrace)
	return httpClient
}
