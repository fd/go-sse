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
	Send(event *Event) error
	Close() error
}

type event_writer_t struct {
	conn         net.Conn
	buf_writer   *bufio.ReadWriter
	chunk_writer io.WriteCloser
	events       chan send_event
	closed       chan chan error
}

type send_event struct {
	event *Event
	reply chan error
}

// Hijack the http.ResponseWriter. The Content-Type header is set to
// text/event-stream and the status code is set to 200.
// The caller is responsible for Closing the EventWriter.
func Hijack(w http.ResponseWriter) (EventWriter, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)

	conn, bufrw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, err
	}

	bufrw.Flush()

	ew := &event_writer_t{
		conn:         conn,
		buf_writer:   bufrw,
		chunk_writer: httputil.NewChunkedWriter(bufrw),
		events:       make(chan send_event),
		closed:       make(chan chan error),
	}

	go ew.loop()

	return ew, nil
}

func (ew *event_writer_t) Send(event *Event) error {
	c := make(chan error)
	ew.events <- send_event{event, c}
	return <-c

}
func (ew *event_writer_t) Close() error {
	c := make(chan error)
	ew.closed <- c
	return <-c
}

func (ew *event_writer_t) loop() {
	var (
		next_keep_alive = time.NewTicker(1 * time.Second)
		zero            = make([]byte, 1)
	)

	for {
		var (
			now = time.Now()
			err error
		)

		ew.conn.SetDeadline(now.Add(5 * time.Second))

		select {

		case req := <-ew.closed:
			next_keep_alive.Stop()
			ew.chunk_writer.Close()
			ew.buf_writer.Flush()
			ew.conn.Close()
			close(ew.closed)
			close(ew.events)
			req <- nil
			return

		case req := <-ew.events:
			err = req.event.write_to(ew.chunk_writer)
			if err == io.EOF {
				go ew.Close()
			}

			err = ew.buf_writer.Flush()
			if err == io.EOF {
				go ew.Close()
			}
			req.reply <- err

		case <-next_keep_alive.C:
			ew.conn.SetReadDeadline(now)
			if _, err := ew.conn.Read(zero); err == io.EOF {
				go ew.Close()
			}

			_, err = ew.chunk_writer.Write(keep_alive_payload)
			if err == io.EOF {
				go ew.Close()
			}
			ew.buf_writer.Flush()

		}
	}
}
