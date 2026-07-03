package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalesfsp/sypl/fields"
	"go.elastic.co/apm"
)

func TestGet_IsSingleton(t *testing.T) {
	a := Get()
	b := Get()
	require.NotNil(t, a)
	assert.Same(t, a, b, "Get must return the same logger instance")
	assert.NotNil(t, a.GetOutputs())
}

func TestToAPM_NilFieldsWithoutTransaction(t *testing.T) {
	// No APM transaction in the context → fields returned without trace keys.
	f := ToAPM(context.Background(), nil)
	require.NotNil(t, f)
	assert.NotContains(t, f, "trace.id")
	assert.NotContains(t, f, "transaction.id")
}

func TestToAPM_PreservesExistingFields(t *testing.T) {
	in := fields.Fields{"k": "v"}
	out := ToAPM(context.Background(), in)
	assert.Equal(t, "v", out["k"])
}

func TestToAPM_WithTransactionAndSpan(t *testing.T) {
	tracer, err := apm.NewTracer("test", "0.0.0")
	require.NoError(t, err)
	defer tracer.Close()

	tx := tracer.StartTransaction("tx", "test")
	defer tx.End()

	ctx := apm.ContextWithTransaction(context.Background(), tx)
	span, ctx := apm.StartSpan(ctx, "span", "custom")
	defer span.End()

	f := ToAPM(ctx, nil)
	assert.Contains(t, f, "trace.id")
	assert.Contains(t, f, "transaction.id")
	assert.Contains(t, f, "span.id")
}
