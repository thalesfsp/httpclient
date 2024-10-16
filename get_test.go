package httpclient

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/thalesfsp/httpclient/v2/internal/shared"
)

func TestClient_Get(t *testing.T) {
	type args struct {
		opts []Func
	}
	tests := []struct {
		name                 string
		args                 args
		expectedBody         string
		expectedStatusCode   int
		testServerBody       string
		testServerStatusCode int
		timeout              time.Duration
		want                 *http.Response
		wantErr              bool
	}{
		{
			name:                 "TestClient_Get - should work",
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusOK),
			testServerStatusCode: http.StatusOK,
			timeout:              15 * time.Second,
			want:                 nil,
			wantErr:              false,
		},
		{
			name:                 "TestClient_Get - should work",
			expectedBody:         http.StatusText(http.StatusOK),
			expectedStatusCode:   http.StatusOK,
			testServerBody:       http.StatusText(http.StatusBadRequest),
			testServerStatusCode: http.StatusBadRequest,
			timeout:              15 * time.Second,
			want:                 nil,
			wantErr:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := shared.CreateHTTPTestServer(tt.testServerStatusCode, nil, nil, tt.testServerBody)
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			c, err := NewDefault(WithClientName("test"))
			if err != nil {
				t.Fatalf("NewDefault() error = %v", err)
			}

			got, err := c.Get(ctx, server.URL, tt.args.opts...)

			if err != nil && !tt.wantErr {
				t.Fatalf("err = %v, want %v", err, nil)
			}

			if got == nil && !tt.wantErr {
				t.Fatalf("got = %v, want %v", got, nil)
			}

			if got != nil {
				if got.StatusCode != tt.expectedStatusCode && !tt.wantErr {
					t.Fatalf("resp.StatusCode = %v, want %v", got.StatusCode, tt.expectedStatusCode)
				}

				body, err := io.ReadAll(got.Body)
				defer got.Body.Close()

				if err != nil {
					t.Fatalf("io.ReadAll(got.Body) error = %v", err)
				}

				if string(body) != tt.expectedBody && !tt.wantErr {
					t.Fatalf("string(body) = %v, want %v", string(body), tt.expectedBody)
				}
			}
		})
	}
}

func TestClient_Get_RespBody(t *testing.T) {
	tests := []struct {
		name                 string
		expectedBody         string
		expectedStatusCode   int
		testServerBody       string
		testServerStatusCode int
		timeout              time.Duration
		want                 *http.Response
		wantErr              bool
	}{
		{
			name:               "TestClient_Get - should work",
			expectedBody:       http.StatusText(http.StatusOK),
			expectedStatusCode: http.StatusOK,
			testServerBody: func() string {
				b, err := shared.Marshal(shared.TestData)
				if err != nil {
					t.Fatalf("shared.Marshal(shared.TestData) error = %v", err)
				}

				return string(b)
			}(),
			testServerStatusCode: http.StatusOK,
			timeout:              10 * time.Second,
			want:                 nil,
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup built-in HTTP test server.
			server := shared.CreateHTTPTestServer(tt.testServerStatusCode, nil, nil, tt.testServerBody)
			defer server.Close()

			// Setup context with timeout.
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Retrieve client - it will be automatically setup.
			c, err := NewDefault(WithClientName("test"))
			if err != nil {
				t.Fatalf("NewDefault() error = %v", err)
			}

			// Get will automagically fill `testData` with the response.
			var testData shared.TestDataS

			got, err := c.Get(ctx, server.URL, WithRespBody(&testData))
			if err != nil && !tt.wantErr {
				t.Fatalf("err = %v, want %v", err, nil)
			}

			defer got.Body.Close()

			//////
			// Assertations.
			//////

			if err != nil && !tt.wantErr {
				t.Fatalf("err = %v, want %v", err, nil)
			}

			if got == nil && !tt.wantErr {
				t.Fatalf("got = %v, want %v", got, nil)
			}

			if got != nil {
				if got.StatusCode != tt.expectedStatusCode && !tt.wantErr {
					t.Fatalf("resp.StatusCode = %v, want %v", got.StatusCode, tt.expectedStatusCode)
				}

				if testData.Name != shared.TestData.Name && testData.Version != shared.TestData.Version {
					t.Fatalf("testData.Name = %v / %v, testData.Version = %v / %v",
						testData.Name,
						shared.TestData.Name,
						testData.Version,
						shared.TestData.Version,
					)
				}
			}
		})
	}
}

func TestClient_Get_ClientWithOptions(t *testing.T) {
	tests := []struct {
		name                 string
		expectedBody         string
		expectedStatusCode   int
		testServerBody       string
		testServerStatusCode int
		timeout              time.Duration
		want                 *http.Response
		wantErr              bool
	}{
		{
			name:               "TestClient_Get - should work",
			expectedBody:       http.StatusText(http.StatusOK),
			expectedStatusCode: http.StatusOK,
			testServerBody: func() string {
				b, err := shared.Marshal(shared.TestData)
				if err != nil {
					t.Fatalf("shared.Marshal(shared.TestData) error = %v", err)
				}

				return string(b)
			}(),
			testServerStatusCode: http.StatusOK,
			timeout:              10 * time.Second,
			want:                 nil,
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup built-in HTTP test server.
			server := shared.CreateHTTPTestServer(
				tt.testServerStatusCode,
				map[string]string{
					"X-Test":   "test",
					"X-Test-2": "test2",
				},
				map[string]string{
					"name":    "john",
					"surname": "peter",
				},
				tt.testServerBody)
			defer server.Close()

			// Setup context with timeout.
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Retrieve client - it will be automatically setup.
			c, err := NewDefault(
				WithClientName("test"),
				WithClientHeader("X-Test", "test"),
				WithClientQueryParam("name", "john"),
			)
			if err != nil {
				t.Fatalf("NewDefault() error = %v", err)
			}

			// Get will automagically fill `testData` with the response.
			var testData shared.TestDataS

			got, err := c.Get(ctx, server.URL,
				WithQueryParam("surname", "peter"),
				WithHeader("X-Test-2", "test2"),
				WithRespBody(&testData),
			)
			if err != nil && !tt.wantErr {
				t.Fatalf("err = %v, want %v", err, nil)
			}

			defer got.Body.Close()

			//////
			// Assertations.
			//////

			if err != nil && !tt.wantErr {
				t.Fatalf("err = %v, want %v", err, nil)
			}

			if got == nil && !tt.wantErr {
				t.Fatalf("got = %v, want %v", got, nil)
			}

			if got != nil {
				if got.StatusCode != tt.expectedStatusCode && !tt.wantErr {
					t.Fatalf("resp.StatusCode = %v, want %v", got.StatusCode, tt.expectedStatusCode)
				}

				if testData.Name != shared.TestData.Name && testData.Version != shared.TestData.Version {
					t.Fatalf("testData.Name = %v / %v, testData.Version = %v / %v",
						testData.Name,
						shared.TestData.Name,
						testData.Version,
						shared.TestData.Version,
					)
				}
			}
		})
	}
}
