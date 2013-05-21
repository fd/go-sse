package sse

import (
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
		err error
		n   int
		m   int
	)

	if e.is_zero() {
		return n, nil
	}

	if e.Id != "" {
		m, err = fmt.Fprintf(w, "id: %s\n", e.Id)
		n += m
		if err != nil {
			return n, err
		}
	}

	if e.Type != "" {
		m, err = fmt.Fprintf(w, "event: %s\n", e.Type)
		n += m
		if err != nil {
			return n, err
		}
	}

	if e.Retry > 0 {
		m, err = fmt.Fprintf(w, "retry: %d\n", e.Retry/time.Millisecond)
		n += m
		if err != nil {
			return n, err
		}
	}

	for _, line := range strings.Split(e.Data, "\n") {
		m, err = fmt.Fprintf(w, "data: %s\n", line)
		n += m
		if err != nil {
			return n, err
		}
	}

	m, err = fmt.Fprint(w, "\n")
	n += m
	if err != nil {
		return n, err
	}

	return n, nil
}
