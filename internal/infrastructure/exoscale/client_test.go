package exoscale

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func Test_metadataRoundTripper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        int
		err           error
		wantMessage   string
		wantHTTPError string
	}{
		{name: "success", status: http.StatusAccepted, wantMessage: "-> 202 Accepted"},
		{name: "HTTP error", status: http.StatusConflict, wantMessage: "-> 409 Conflict", wantHTTPError: "Conflict"},
		{name: "request error", err: errors.New("request failed for https://api.example.test/v2/instance?token=query-secret"), wantMessage: "-> REQUEST ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output string
			logger := funcr.New(func(prefix, args string) { output += prefix + args }, funcr.Options{})
			transport := metadataRoundTripper{
				logger: logger,
				next: roundTripFunc(func(*http.Request) (*http.Response, error) {
					if tc.err != nil {
						return nil, tc.err
					}
					return &http.Response{StatusCode: tc.status, Body: http.NoBody}, nil
				}),
			}
			req, err := http.NewRequest(http.MethodPost, "https://api.example.test/v2/instance?token=query-secret", strings.NewReader("body-secret"))
			assert.NoError(t, err)
			req.Header.Set("Authorization", "header-secret")

			resp, err := (&http.Client{Transport: transport}).Do(req)

			assert.ErrorIs(t, err, tc.err)
			if tc.err == nil {
				assert.Equal(t, tc.status, resp.StatusCode)
			} else {
				assert.Nil(t, resp)
				assert.NotContains(t, err.Error(), "query-secret")
			}
			assert.Contains(t, output, "HTTP POST api.example.test/v2/instance "+tc.wantMessage)
			assert.Contains(t, output, `"method"="POST"`)
			assert.Contains(t, output, `"host"="api.example.test"`)
			assert.Contains(t, output, `"path"="/v2/instance"`)
			assert.Contains(t, output, fmt.Sprintf(`"status"=%d`, tc.status))
			assert.Contains(t, output, `"duration"=`)
			if tc.wantHTTPError != "" {
				assert.Contains(t, output, `"httpError"="`+tc.wantHTTPError+`"`)
			}
			assert.NotContains(t, output, "query-secret")
			assert.NotContains(t, output, "body-secret")
			assert.NotContains(t, output, "header-secret")
			assert.NotContains(t, output, "Authorization")
		})
	}
}

func Test_New_rejectsIncompleteCredentials(t *testing.T) {
	t.Parallel()

	client, err := New("key", "", egoscale.ZoneNameCHGva2)

	assert.Nil(t, client)
	assert.ErrorIs(t, err, credentials.ErrMissingIncomplete)
	assert.ErrorContains(t, err, "unable to create exoscale client")
}
