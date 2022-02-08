package pipe

import (
	"context"
	"testing"
	"time"

	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeout = 5 * time.Second

func TestPipe(t *testing.T) {
	ingest := make(chan string, 5)
	defer close(ingest)
	submit := make(chan flow.Record)
	defer close(submit)
	pipe := New(
		// test ingester
		func(ctx context.Context) <-chan flow.Record {
			out := make(chan flow.Record)
			go func() {
				for {
					select {
					case <-ctx.Done():
						close(out)
						return
					case i := <-ingest:
						out <- flow.Record{"key": []string{i}}
					}
				}
			}()
			return out
		},
		// test submitter
		func(record flow.Record) {
			record["key"] = append(record["key"].([]string), "submit")
			submit <- record
		},
		// test transformer 1
		func(record flow.Record) flow.Record {
			record["key"] = append(record["key"].([]string), "one")
			return record
		},
		// test transformer 2
		func(record flow.Record) flow.Record {
			record["key"] = append(record["key"].([]string), "two")
			return record
		},
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pipe.Start(ctx)
	// WHEN something enters into the pipeline
	ingest <- "ingest"
	// THEN it has been processed and submitted in order
	select {
	case msg := <-submit:
		assert.Equal(t, msg, flow.Record{"key": []string{"ingest", "one", "two", "submit"}})
	case <-time.After(timeout):
		require.Fail(t, "timed out while waiting for the pipeline to process a message")
	}
	// AND WHEN the pipeline is closed
	cancel()
	// THEN no other messages can be processed
	ingest <- "ingest"
	select {
	case out := <-submit:
		require.Failf(t, "unexpected message in the pipeline", "%#v", out)
	case <-time.After(20 * time.Millisecond):
		// TEST passed!
	}
}
