// Package consumer defines a Consumer interface for various consumers
package consumer

import "context"

type Consumer interface {
	Start(context context.Context, cancel context.CancelFunc)
}
