package shared

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestGenerateUUID(t *testing.T) {
	a := GenerateUUID()
	b := GenerateUUID()
	assert.Len(t, a, 36)
	assert.NotEqual(t, a, b)
}

func TestGenerateID(t *testing.T) {
	// Deterministic and whitespace-insensitive.
	assert.Equal(t, GenerateID("hello"), GenerateID("  hello \n"))
	assert.NotEqual(t, GenerateID("a"), GenerateID("b"))
	assert.Len(t, GenerateID("x"), 64) // sha-256 hex
}

func TestSliceContains(t *testing.T) {
	src := []string{"Alpha", "Beta"}
	assert.True(t, SliceContains(src, "alpha")) // case-insensitive
	assert.True(t, SliceContains(src, "BETA"))
	assert.False(t, SliceContains(src, "gamma"))
	assert.False(t, SliceContains(nil, "x"))
}

func TestMarshalUnmarshal(t *testing.T) {
	type payload struct {
		A string `json:"a"`
	}

	b, err := Marshal(payload{A: "v"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"v"}`, string(b))

	var out payload
	require.NoError(t, Unmarshal(b, &out))
	assert.Equal(t, "v", out.A)

	// Error paths.
	_, err = Marshal(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")

	err = Unmarshal([]byte("{not-json"), &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestDecode(t *testing.T) {
	var out struct {
		A string `json:"a"`
	}
	require.NoError(t, Decode(strings.NewReader(`{"a":"v"}`), &out))
	assert.Equal(t, "v", out.A)

	require.Error(t, Decode(strings.NewReader("{bad"), &out))
}

func TestReadAll(t *testing.T) {
	b, err := ReadAll(strings.NewReader("data"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(b))

	_, err = ReadAll(errReader{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read response body")
}

func TestHasLetter(t *testing.T) {
	assert.True(t, HasLetter("abc123"))
	assert.True(t, HasLetter("123a"))
	assert.False(t, HasLetter("12345"))
	assert.False(t, HasLetter(""))
}

func TestLettersToPhoneNumberKeys(t *testing.T) {
	assert.Equal(t, "362867", LettersToPhoneNumberKeys("DOCTOR"))
	assert.Equal(t, "362867", LettersToPhoneNumberKeys("doctor"))
}

func TestTargetName(t *testing.T) {
	n, err := TargetName("primary", "alt")
	require.NoError(t, err)
	assert.Equal(t, "primary", n)

	n, err = TargetName("", "alt")
	require.NoError(t, err)
	assert.Equal(t, "alt", n)

	_, err = TargetName("", "")
	require.Error(t, err)
}

func TestHTTPMethodString(t *testing.T) {
	assert.Equal(t, "GET", MethodGet.String())
	assert.Equal(t, "POST", MethodPost.String())
	assert.Equal(t, "DELETE", MethodDelete.String())
}
