package httpclient

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestInitialize_withOptions(t *testing.T) {
	once = sync.Once{}

	got, err := Initialize(WithClientName("testclientname"), WithPrefix("testprefix"))
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	name := got.GetName()

	assert.NoError(t, err)
	assert.Equal(t, name, "testclientname")
}

func TestInitialize(t *testing.T) {
	once = sync.Once{}

	got, err := Initialize()
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	name := got.GetName()

	assert.NoError(t, err)
	assert.Contains(t, name, "httpclient")
	assert.Len(t, name, 47)
}
