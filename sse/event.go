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

func (e *Event) write_to(w io.Writer) error {
	var (
		err error
	)

	if e.is_zero() {
		return nil
	}

	if e.Id != "" {
		_, err = fmt.Fprintf(w, "id: %s\n", e.Id)
		if err != nil {
			return err
		}
	}

	if e.Type != "" {
		_, err = fmt.Fprintf(w, "event: %s\n", e.Type)
		if err != nil {
			return err
		}
	}

	if e.Retry > 0 {
		_, err = fmt.Fprintf(w, "retry: %d\n", e.Retry/time.Millisecond)
		if err != nil {
			return err
		}
	}

	for _, line := range strings.Split(e.Data, "\n") {
		_, err = fmt.Fprintf(w, "data: %s\n", line)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprint(w, "\n")
	if err != nil {
		return err
	}

	return nil
}
