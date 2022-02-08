// Package pipe provides the internal asynchronous pipeline functionality
package pipe

import (
	"context"

	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
)

// TODO: make configurable
const (
	ChannelsBuffer = 20
)

// Ingester acquires flow records from the outside (IPFIX UDP collector, Kafka, etc...),
// transforms them to the flow.Record format, and forwards them to the next pipeline stage
// It can be stopped via the passed context.
// Implementors must spawn their own goroutine and close the returned channel when they are
// stopped.
type Ingester func(ctx context.Context) <-chan flow.Record

// Submitter forwards flow records to the outside (e.g. Loki, Kafka, etc...). It will be stopped
// if the input channel is closed
type Submitter func(flow.Record)

// Transformer receives flows, transforms them, and forwards them to the next stage of the pipeline.
// It will be stopped if the input channel is closed
type Transformer func(flow.Record) flow.Record

// Pipeline structure
// TODO: support multiple ingesters and submitters
// TODO: support multiple workers per stage
// ingester --> transformers... --> submitter
type Pipeline struct {
	ingester     Ingester
	transformers []Transformer
	submitter    Submitter
}

func New(in Ingester, out Submitter, inBetween ...Transformer) *Pipeline {
	return &Pipeline{
		ingester:     in,
		transformers: inBetween,
		submitter:    out,
	}
}

func (p *Pipeline) Start(ctx context.Context) {
	ch := p.ingester(ctx)
	ch = p.startTransformers(ch)
	p.startSubmitter(ch)
}

// connects all the transformers in the given order
func (p *Pipeline) startTransformers(in <-chan flow.Record) <-chan flow.Record {
	for _, transformer := range p.transformers {
		in = p.backgroundTransform(in, transformer)
	}
	return in
}

func (p *Pipeline) backgroundTransform(
	in <-chan flow.Record, transform Transformer) <-chan flow.Record {

	out := make(chan flow.Record, ChannelsBuffer)
	go func() {
		for input := range in {
			out <- transform(input)
		}
		close(out)
	}()
	return out
}

func (p *Pipeline) startSubmitter(in <-chan flow.Record) {
	go func() {
		for input := range in {
			p.submitter(input)
		}
	}()
}
