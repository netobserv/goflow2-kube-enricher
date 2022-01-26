// Package reader reads input flows
package reader

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/netobserv/goflow2-kube-enricher/pkg/enricher"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
)

type Reader struct {
	log      *logrus.Entry
	format   format.Format
	enricher *enricher.Enricher
}

func NewReader(format format.Format, log *logrus.Entry, enricher enricher.Enricher) Reader {
	return Reader{
		log:      log,
		format:   format,
		enricher: &enricher,
	}
}

func (r *Reader) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.format.Shutdown()
			return
		default:
			record, err := r.format.Next()
			if err != nil {
				r.log.Error(err)
				return
			}
			if record == nil {
				r.log.Error("nil record")
				return
			}
			if r.enricher != nil {
				err = r.enricher.Enrich(record)
				if err != nil {
					r.log.Error(err)
				}
			} else {
				r.log.Error("nil enricher")
			}
		}
	}
}
