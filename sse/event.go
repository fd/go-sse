package sse

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

type Event struct {
	Id    string
	Type  string
	Data  string
	Retry time.Duration
}

func (e *Event) is_zero() bool {
	return e.Id == "" && e.Type == "" && e.Data == "" && e.Retry == 0
}

func (e *Event) write_to(w io.Writer) (int, error) {
	var (
		buf bytes.Buffer
		err error
	)

	if e.is_zero() {
		return 0, nil
	}

	if e.Id != "" {
		_, err = fmt.Fprintf(&buf, "id: %s\n", e.Id)
		if err != nil {
			return 0, err
		}
	}

	if e.Type != "" {
		_, err = fmt.Fprintf(&buf, "event: %s\n", e.Type)
		if err != nil {
			return 0, err
		}
	}

	if e.Retry > 0 {
		_, err = fmt.Fprintf(&buf, "retry: %d\n", e.Retry/time.Millisecond)
		if err != nil {
			return 0, err
		}
	}

	for _, line := range strings.Split(e.Data, "\n") {
		_, err = fmt.Fprintf(&buf, "data: %s\n", line)
		if err != nil {
			return 0, err
		}
	}

	_, err = fmt.Fprint(&buf, "\n")
	if err != nil {
		return 0, err
	}

	return w.Write(buf.Bytes())
}
