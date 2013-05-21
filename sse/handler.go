package sse

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"time"
)

var keep_alive_payload = []byte(":keep-alive\n" +
	":xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n" +
	":xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n" +
	":xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n" +
	":xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n\n")

type EventWriter interface {
	LastEventID() string
	Write(event *Event) (n int, err error)
	Close() error
}

type event_writer_t struct {
	last_id      string
	conn         net.Conn
	buf_writer   *bufio.ReadWriter
	chunk_writer io.WriteCloser
	events       chan write_event_req
	closed       chan chan error
	err          error
}

type write_event_req struct {
	event *Event
	reply chan write_event_res
}

type write_event_res struct {
	n   int
	err error
}

// Hijack the http.ResponseWriter. The Content-Type header is set to
// text/event-stream and the status code is set to 200.
// The caller is responsible for Closing the EventWriter.
func Hijack(w http.ResponseWriter, req *http.Request) (EventWriter, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)

	conn, bufrw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, err
	}

	bufrw.Flush()

	ew := &event_writer_t{
		last_id:      req.Header.Get("Last-Event-ID"),
		conn:         conn,
		buf_writer:   bufrw,
		chunk_writer: httputil.NewChunkedWriter(bufrw),
		events:       make(chan write_event_req),
		closed:       make(chan chan error),
	}

	go ew.loop()

	return ew, nil
}

func (ew *event_writer_t) LastEventID() string {
	return ew.last_id
}

func (ew *event_writer_t) Write(event *Event) (int, error) {
	c := make(chan write_event_res)
	ew.events <- write_event_req{event, c}
	res := <-c
	return res.n, res.err
}

func (ew *event_writer_t) Close() error {
	c := make(chan error)
	ew.closed <- c
	return <-c
}

func (ew *event_writer_t) loop() {
	var (
		next_keep_alive = time.NewTicker(1 * time.Second)
	)

	defer func() {
		next_keep_alive.Stop()
		ew.chunk_writer.Close()
		ew.buf_writer.Flush()
		ew.conn.Close()
		close(ew.closed)
		close(ew.events)
	}()

	for {
		var (
			now = time.Now()
			n   int
			err error
		)

		ew.conn.SetDeadline(now.Add(5 * time.Second))

		select {

		case req := <-ew.closed:
			req <- ew.err
			return

		case req := <-ew.events:
			n, err = ew.write_event(req.event)
			if err == io.EOF {
				go ew.Close()
			}

			req.reply <- write_event_res{n, err}

		case <-next_keep_alive.C:
			err = ew.write_keep_alive()
			if err == io.EOF {
				go ew.Close()
			}
			ew.err = err

		}
	}
}

func (ew *event_writer_t) write_event(event *Event) (int, error) {
	var (
		err error
		n   int
	)

	// write the message
	n, err = event.write_to(ew.chunk_writer)
	if err != nil {
		return n, err
	}

	// flush the buffer
	err = ew.buf_writer.Flush()
	if err != nil {
		return n, err
	}

	return n, nil
}

func (ew *event_writer_t) write_keep_alive() error {
	var (
		now  = time.Now()
		zero = make([]byte, 1)
		err  error
	)

	// did the client hang up?
	ew.conn.SetReadDeadline(now)
	if _, err := ew.conn.Read(zero); err == io.EOF {
		return err
	}

	// send the keep alive
	_, err = ew.chunk_writer.Write(keep_alive_payload)
	if err != nil {
		return err
	}

	// flush the buffer
	err = ew.buf_writer.Flush()
	if err != nil {
		return err
	}

	return nil
}
