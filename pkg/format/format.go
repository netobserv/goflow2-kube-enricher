// Package format defines a Format interface for various input formats
package format

import "context"

type Format interface {
	Next() (map[string]interface{}, error)
	Start(ctx context.Context)
	Shutdown()
}
