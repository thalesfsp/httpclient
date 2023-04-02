package httpclient

import (
	"net/http"
	"testing"
)

func TestIsRespSuccess(t *testing.T) {
	// create a new http.Response with a success status code
	resp := &http.Response{
		StatusCode: http.StatusOK,
	}

	// check if the response is a success
	if !IsRespSuccess(resp) {
		t.Errorf("expected response with status code %v to be a success", resp.StatusCode)
	}

	// create a new http.Response with a non-success status code
	resp = &http.Response{
		StatusCode: http.StatusBadRequest,
	}

	// check if the response is a success
	if IsRespSuccess(resp) {
		t.Errorf("expected response with status code %v to not be a success", resp.StatusCode)
	}
}
