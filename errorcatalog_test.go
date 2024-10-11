package httpclient

import (
	"net/http"
	"testing"
)

func TestIsErrorStatusCode(t *testing.T) {
	type args struct {
		statusCode int
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should return true",
			args: args{
				statusCode: http.StatusUnprocessableEntity,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ErrorByStatusCode(tt.args.statusCode)

			if got := IsErrorStatusCode(err, tt.args.statusCode); got != tt.want {
				t.Errorf("IsErrorStatusCode() = %v, want %v", got, tt.want)
			}
		})
	}
}
