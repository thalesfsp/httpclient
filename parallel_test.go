package httpclient

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thalesfsp/httpclient/internal/shared"
)

//nolint:bodyclose
func TestClient_ParallelGet2(t *testing.T) {
	type args struct {
		opts []Func
	}
	tests := []struct {
		args                 args
		count                int
		expectedBody         string
		expectedHeaders      map[string]string
		expectedQueryParams  map[string]string
		expectedStatusCode   int
		name                 string
		testServerBody       string
		testServerStatusCode int
		timeout              time.Duration
		url                  string
		wantErr              bool
	}{
		{
			name:                 "TestClient_ParallelGet - should work",
			args:                 args{},
			count:                3,
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusOK),
			testServerStatusCode: http.StatusOK,
			timeout:              3 * time.Second,
			wantErr:              false,
		},
		{
			name: "TestClient_ParallelGet - should work - Options",
			args: args{
				opts: []Func{
					WithHeader("X-Test", "test"),
					WithQueryParam("name", "john"),
					WithBasicAuth("john", "doe"),
				},
			},
			count: 1,

			expectedBody: http.StatusText(http.StatusOK),
			expectedHeaders: map[string]string{
				"X-Test":        "test",
				"Authorization": "Basic am9objpkb2U=",
			},
			expectedQueryParams: map[string]string{"name": "john"},
			expectedStatusCode:  http.StatusOK,

			testServerBody:       http.StatusText(http.StatusOK),
			testServerStatusCode: http.StatusOK,
			timeout:              3 * time.Second,
			wantErr:              false,
		},
		{
			name:                 "TestClient_ParallelGet - should fail, and trigger retry - StatusBadGateway",
			args:                 args{},
			count:                3,
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusBadGateway),
			testServerStatusCode: http.StatusBadGateway,
			timeout:              15 * time.Second,
			wantErr:              true,
		},
		{
			name:                 "TestClient_ParallelGet - should fail, and trigger retry - StatusTooManyRequests",
			args:                 args{},
			count:                3,
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusTooManyRequests),
			testServerStatusCode: http.StatusTooManyRequests,
			timeout:              15 * time.Second,
			wantErr:              true,
		},
		{
			name:                 "TestClient_ParallelGet - should fail, and not trigger retry - StatusBadRequest",
			args:                 args{},
			count:                3,
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusBadRequest),
			testServerStatusCode: http.StatusBadRequest,
			timeout:              15 * time.Second,
			wantErr:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := shared.CreateHTTPTestServer(
				tt.testServerStatusCode,
				tt.expectedHeaders,
				tt.expectedQueryParams,
				tt.testServerBody,
			)
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			urls := []string{}
			for i := 0; i < tt.count; i++ {
				urls = append(urls, server.URL)
			}

			if tt.url != "" {
				urls = []string{tt.url}
			}

			_, err := NewDefaultSingleton("parallelget2")
			assert.NoError(t, err)

			responses, errors := Get().ParallelGet(ctx, tt.args.opts, urls...)
			if errors != nil && !tt.wantErr {
				t.Fatalf("Errors = %v, want %v", shared.PrintErrorMessages(errors...), nil)
			}

			for _, r := range responses {
				if r == nil && !tt.wantErr {
					t.Fatalf("r = %v, want %v", r, nil)
				}

				if r != nil {
					defer r.Body.Close()

					if r.StatusCode != tt.expectedStatusCode && !tt.wantErr {
						t.Fatalf("resp.StatusCode = %v, want %v", r.StatusCode, tt.expectedStatusCode)
					}

					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatalf("io.ReadAll(r.Body) error = %v", err)
					}

					defer r.Body.Close()

					if string(body) != tt.expectedBody && !tt.wantErr {
						t.Fatalf("string(body) = %v, want %v", string(body), tt.expectedBody)
					}
				}
			}
		})
	}
}
