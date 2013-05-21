package sse

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
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
	CloseNotify() <-chan bool
}

type event_writer_t struct {
	last_id         string
	conn            net.Conn
	closed          chan bool
	close_notifiers []chan bool
	mtx             sync.RWMutex
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
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, err
	}

	ew := &event_writer_t{
		last_id: req.Header.Get("Last-Event-ID"),
		conn:    conn,
		closed:  make(chan bool),
	}

	err = ew.write_response()
	if err != nil {
		ew.conn.Close()
		return nil, err
	}

	go ew.loop()

	return ew, nil
}

func (ew *event_writer_t) LastEventID() string {
	return ew.last_id
}

func (ew *event_writer_t) Write(event *Event) (int, error) {
	ew.mtx.Lock()
	defer ew.mtx.Unlock()

	return event.write_to(ew.conn)
}

func (ew *event_writer_t) Close() error {
	ew.mtx.Lock()
	defer ew.mtx.Unlock()

	if ew.closed != nil {
		select {
		case ew.closed <- true:
		default:
		}
		close(ew.closed)
		ew.closed = nil
	}

	for _, c := range ew.close_notifiers {
		c <- true
		close(c)
	}
	ew.close_notifiers = nil

	return ew.conn.Close()
}

func (ew *event_writer_t) CloseNotify() <-chan bool {
	ew.mtx.Lock()
	defer ew.mtx.Unlock()

	c := make(chan bool, 1)
	if ew.closed == nil {
		c <- true
		close(c)
		return c
	}

	ew.close_notifiers = append(ew.close_notifiers, c)
	return c
}

func (ew *event_writer_t) loop() {
	var (
		next_keep_alive = time.NewTicker(1 * time.Second)
		zero            = make([]byte, 1)
		closed          = ew.closed
	)

	defer func() {
		next_keep_alive.Stop()
	}()

	for {
		var (
			now = time.Now()
		)

		ew.conn.SetDeadline(now.Add(5 * time.Second))

		select {

		case <-closed:
			// closed on local side
			return

		case <-next_keep_alive.C:
			// did the client hang up?
			ew.conn.SetReadDeadline(time.Now())
			if _, err := ew.conn.Read(zero); err == io.EOF {
				// closed on remote side
				ew.Close()
			} else {
				fmt.Printf("err: %T %s\n", err, err)
				// send the keep alive
				ew.conn.SetDeadline(now.Add(5 * time.Second))
				ew.conn.Write(keep_alive_payload)
			}

		}
	}
}

func (ew *event_writer_t) write_response() error {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header, 10),
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Set("Connection", "close")
	resp.Header.Set("Date", time.Now().Format(http.TimeFormat))

	return resp.Write(ew.conn)
}
