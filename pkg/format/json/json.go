// Package json defines a json Format, implementing Format interface for input
package json

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	"github.com/netobserv/goflow2-kube-enricher/pkg/pipe"
	"github.com/sirupsen/logrus"
)

var jlog = logrus.WithField("component", "json.Scanner")

// Ingester returns a pipe.Ingester instance that reads the JSON flows from the input Reader
// and forwards them to an output channel
func Ingester(in io.Reader) pipe.Ingester {
	return func(ctx context.Context) <-chan flow.Record {
		out := make(chan flow.Record, pipe.ChannelsBuffer)
		go scanLines(ctx, in, out)
		return out
	}
}

func scanLines(ctx context.Context, in io.Reader, out chan<- flow.Record) {
	scanner := bufio.NewScanner(in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if record, err := next(scanner); err != nil {
				if errors.Is(err, io.EOF) {
					jlog.Info("reached end of input. Stopping")
					return
				}
				jlog.WithError(err).Warn("can't read record")
			} else {
				out <- record
			}
		}
	}
}

func next(scanner *bufio.Scanner) (flow.Record, error) {
	if scanner.Scan() {
		raw := scanner.Bytes()
		var record map[string]interface{}
		err := json.Unmarshal(raw, &record)
		if err != nil {
			return nil, err
		}
		return record, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// if the scanner reaches the end, it returns a nil error
	// so we override to know we reached the end
	return nil, io.EOF
}
