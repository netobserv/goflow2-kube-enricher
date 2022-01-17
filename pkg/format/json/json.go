// Package json defines a json Format, implementing Format interface for input
package json

import (
	"bufio"
	"encoding/json"
	"io"
)

type Format struct {
	scanner *bufio.Scanner
}

func NewScanner(in io.Reader) *Format {
	input := Format{}
	input.scanner = bufio.NewScanner(in)
	return &input
}

func (j *Format) Next() (map[string]interface{}, error) {
	if j.scanner.Scan() {
		raw := j.scanner.Bytes()
		var record map[string]interface{}
		err := json.Unmarshal(raw, &record)
		if err != nil {
			return nil, err
		}
		return record, nil
	}
	err := j.scanner.Err()
	return nil, err
}

func (j *Format) Shutdown() {
	//Can't shutdown json formatter
}
