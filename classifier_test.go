package httpclient

import (
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/thalesfsp/customerror"
)

func TestHTTPStatusCodeClassifier_Classify(t *testing.T) {
	testRegex := regexp.MustCompile(`(?m)^test`)

	type fields struct {
		CustomRegexes    []*regexp.Regexp
		HTTPRetrierRegex *regexp.Regexp
		StatusCodes      []int
	}
	type args struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   retrier.Action
	}{
		{
			name: "should return retrier.Succeed",
			fields: fields{
				StatusCodes: []int{400},
			},
			args: args{
				err: nil,
			},
			want: retrier.Succeed,
		},
		{
			name: "should return retrier.Retry - status code",
			fields: fields{
				StatusCodes: []int{400},
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 400},
			},
			want: retrier.Retry,
		},
		{
			name: "should return retrier.Fail - not in StatusCodes",
			fields: fields{
				StatusCodes: []int{400},
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 401},
			},
			want: retrier.Fail,
		},
		{
			name: "should return retrier.Retry - regex",
			fields: fields{
				HTTPRetrierRegex: httpRetrierRegex,
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 429},
			},
			want: retrier.Retry,
		},
		{
			name: "should return retrier.Fail - regex",
			fields: fields{
				HTTPRetrierRegex: httpRetrierRegex,
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 400},
			},
			want: retrier.Fail,
		},
		{
			name: "should return retrier.Retry - any regex",
			fields: fields{
				CustomRegexes: []*regexp.Regexp{testRegex},
			},
			args: args{
				err: &customerror.CustomError{Message: "test"},
			},
			want: retrier.Retry,
		},
		{
			name: "should return retrier.Fail - not a custom error",
			fields: fields{
				StatusCodes: []int{400},
			},
			args: args{
				err: errors.New("test"),
			},
			want: retrier.Fail,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hSCC := CustomClassifier{
				CustomRegexes:    tt.fields.CustomRegexes,
				HTTPRetrierRegex: tt.fields.HTTPRetrierRegex,
				StatusCodes:      tt.fields.StatusCodes,
			}
			if err := hSCC.Classify(tt.args.err); !reflect.DeepEqual(err, tt.want) {
				t.Errorf("HTTPStatusCodeClassifier.Classify() = %v, want %v", err, tt.want)
			}
		})
	}
}
