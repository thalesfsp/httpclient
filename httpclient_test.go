package httpclient

import (
	"net/http"
	"testing"
	"time"

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

func TestNewDefault_withOptions(t *testing.T) {
	singleton.Store(nil)

	got, err := NewDefault(WithClientName("testclientname"), WithPrefix("testprefix"))
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}

	name := got.GetName()

	assert.NoError(t, err)
	assert.Equal(t, name, "testclientname")
}

func TestNewDefault(t *testing.T) {
	singleton.Store(nil)

	got, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}

	name := got.GetName()

	assert.NoError(t, err)
	assert.Contains(t, name, "httpclient")
	assert.Len(t, name, 47)
}

func TestNewSingleton(t *testing.T) {
	singleton.Store(nil)

	got, err := NewSingleton(
		"test",
		nil,
		nil,
		nil,
		1*time.Second,
		1*time.Second,
		3,
	)
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}

	assert.NotPanics(t, func() {
		Get()
	})

	assert.NotNil(t, got)
	assert.NoError(t, err)
	assert.Contains(t, got.GetName(), "test")
}
