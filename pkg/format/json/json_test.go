package json

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeout = 5 * time.Second

func TestJsonScan(t *testing.T) {
	inRead, inWrite := io.Pipe()
	defer inWrite.Close()
	defer inRead.Close()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// GIVEN a json.Ingester
	out := Ingester(inRead)(ctx)

	// WHEN it receives lines from the input stream
	_, err := inWrite.Write([]byte(`{"foo":"bar","baz":123}` + "\n"))
	require.NoError(t, err)
	_, err = inWrite.Write([]byte(`this won't be parsed` + "\n"))
	require.NoError(t, err)
	_, err = inWrite.Write([]byte(`{"hola":"nen","bon":"jour"}` + "\n"))
	require.NoError(t, err)

	// THEN it forwards them formatted as flow.Record instances (ignoring the invalid lines)
	select {
	case scanned := <-out:
		assert.Equal(t, flow.Record{"foo": "bar", "baz": float64(123)}, scanned)
	case <-time.After(timeout):
		require.Fail(t, "timeout while waiting for a scanned message")
	}
	select {
	case scanned := <-out:
		assert.Equal(t, flow.Record{"hola": "nen", "bon": "jour"}, scanned)
	case <-time.After(timeout):
		require.Fail(t, "timeout while waiting for a scanned message")
	}
	select {
	case scanned := <-out:
		require.Fail(t, "unexpected record", scanned)
	default:
		// OK!
	}

}
