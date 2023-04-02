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
	type fields struct {
		Regex       *regexp.Regexp
		StatusCodes []int
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
				Regex: httpRetrierRegex,
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 429},
			},
			want: retrier.Retry,
		},
		{
			name: "should return retrier.Fail - regex",
			fields: fields{
				Regex: httpRetrierRegex,
			},
			args: args{
				err: &customerror.CustomError{StatusCode: 400},
			},
			want: retrier.Fail,
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
			hSCC := HTTPStatusCodeClassifier{
				Regex:       tt.fields.Regex,
				StatusCodes: tt.fields.StatusCodes,
			}
			if got := hSCC.Classify(tt.args.err); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HTTPStatusCodeClassifier.Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}
