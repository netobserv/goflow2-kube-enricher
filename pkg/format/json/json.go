package json

import (
	"bufio"
	"encoding/json"
	"io"
)

type JsonFormat struct {
	scanner *bufio.Scanner
}

func NewScanner(in io.Reader) *JsonFormat {
	input := JsonFormat{}
	input.scanner = bufio.NewScanner(in)
	return &input
}

func (j *JsonFormat) Next() (map[string]interface{}, error) {
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
