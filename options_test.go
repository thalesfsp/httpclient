package httpclient

import (
	"bytes"
	"io"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/thalesfsp/httpclient/internal/shared"
)

type TestStruct struct {
	A string `json:"a"`
}

func TestWithReqBody(t *testing.T) {
	tests := []struct {
		name     string
		body     interface{}
		expected io.Reader
		err      error
	}{
		{
			name:     "nil body",
			body:     nil,
			expected: nil,
			err:      nil,
		},
		{
			name:     "string body",
			body:     "test",
			expected: strings.NewReader("test"),
			err:      nil,
		},
		{
			name:     "io.Reader body",
			body:     strings.NewReader("test"),
			expected: strings.NewReader("test"),
			err:      nil,
		},
		{
			name: "struct body",
			body: TestStruct{A: "test"},
			expected: func() io.Reader {
				bodyBytes, err := shared.Marshal(TestStruct{A: "test"})
				if err != nil {
					t.Errorf("WithReqBody() error = %v", err)
				}
				return bytes.NewReader(bodyBytes)
			}(),
			err: nil,
		},
		{
			name: "url.Values body",
			body: url.Values{
				"A": {"test"},
			},
			expected: strings.NewReader("A=test"),
			err:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts Options

			err := WithReqBody(tt.body)(&opts)
			if !reflect.DeepEqual(err, tt.err) {
				t.Errorf("WithReqBody() error = %v, wantErr %v", err, tt.err)
			}
			if tt.expected != nil {
				if !reflect.DeepEqual(opts.reqBodyAsIOReader, tt.expected) {
					t.Errorf("WithReqBody() = %v, want %v", opts.reqBodyAsIOReader, tt.expected)
				}
			}
		})
	}
}
