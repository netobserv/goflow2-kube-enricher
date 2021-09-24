package format

type Format interface {
	Next() (map[string]interface{}, error)
}
