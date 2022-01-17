// Package format defines a Format interface for various input formats
package format

type Format interface {
	Next() (map[string]interface{}, error)
	Shutdown()
}
