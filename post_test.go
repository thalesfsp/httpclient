package httpclient

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/thalesfsp/httpclient/internal/shared"
)

func TestClient_Post(t *testing.T) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {"adasad"},
		"redirect_uri":  {"http://localhost:8080"},
		"code":          {"qwdbvd"},
		"client_secret": {"qweqwe"},
	}

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
			name:               "TestClient_Post - should work",
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
			server := shared.CreateHTTPTestServer(tt.testServerStatusCode, map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			}, nil, tt.testServerBody)
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			c, err := NewDefault("test")
			if err != nil {
				t.Fatalf("NewDefault() error = %v", err)
			}

			var testData shared.TestDataS

			got, err := c.Post(ctx, server.URL,
				WithHeader("Content-Type", "application/x-www-form-urlencoded"),
				WithReqBody(strings.NewReader(data.Encode())),
				WithRespBody(&testData),
			)

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
