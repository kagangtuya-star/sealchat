package api

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingWSOutboundSocket struct {
	mu       sync.Mutex
	payloads [][]byte
	wrote    chan struct{}
	entered  chan struct{}
	block    <-chan struct{}
}

func (s *recordingWSOutboundSocket) SetWriteDeadline(time.Time) error {
	return nil
}

func (s *recordingWSOutboundSocket) WriteMessage(_ int, payload []byte) error {
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		<-s.block
	}
	s.mu.Lock()
	s.payloads = append(s.payloads, append([]byte(nil), payload...))
	s.mu.Unlock()
	if s.wrote != nil {
		s.wrote <- struct{}{}
	}
	return nil
}

func (s *recordingWSOutboundSocket) decodedStrings(t *testing.T) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]string, 0, len(s.payloads))
	for _, payload := range s.payloads {
		var value string
		if err := json.Unmarshal(payload, &value); err != nil {
			t.Fatalf("decode outbound payload: %v", err)
		}
		values = append(values, value)
	}
	return values
}

func newTestWsSyncConn(socket wsOutboundSocket, queueSize int) *WsSyncConn {
	c := &WsSyncConn{
		outbound:       make(chan wsOutboundMessage, queueSize),
		done:           make(chan struct{}),
		outboundSocket: socket,
	}
	go c.outboundWriter()
	return c
}

func waitForWrites(t *testing.T, wrote <-chan struct{}, count int) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for range count {
		select {
		case <-wrote:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d websocket writes", count)
		}
	}
}

func TestWsSyncConnFIFO(t *testing.T) {
	socket := &recordingWSOutboundSocket{wrote: make(chan struct{}, 3)}
	c := newTestWsSyncConn(socket, 3)
	defer c.Close()

	for _, value := range []string{"A", "B", "C"} {
		if err := c.EnqueueJSON(value); err != nil {
			t.Fatalf("enqueue %s: %v", value, err)
		}
	}
	waitForWrites(t, socket.wrote, 3)

	got := socket.decodedStrings(t)
	want := []string{"A", "B", "C"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("write order = %#v, want %#v", got, want)
		}
	}
}

func TestWsSyncConnEventBeforeResponse(t *testing.T) {
	socket := &recordingWSOutboundSocket{wrote: make(chan struct{}, 2)}
	c := newTestWsSyncConn(socket, 2)
	defer c.Close()

	if err := c.EnqueueJSON("event"); err != nil {
		t.Fatalf("enqueue event: %v", err)
	}
	responseDone := make(chan error, 1)
	go func() {
		responseDone <- c.WriteJSON("response")
	}()

	waitForWrites(t, socket.wrote, 2)
	if err := <-responseDone; err != nil {
		t.Fatalf("write response: %v", err)
	}
	got := socket.decodedStrings(t)
	if len(got) != 2 || got[0] != "event" || got[1] != "response" {
		t.Fatalf("write order = %#v, want [event response]", got)
	}
}

func TestWsSyncConnQueueFullClosesConnection(t *testing.T) {
	c := &WsSyncConn{
		outbound: make(chan wsOutboundMessage, 1),
		done:     make(chan struct{}),
	}
	if err := c.EnqueueJSON("first"); err != nil {
		t.Fatalf("enqueue first message: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		result <- c.EnqueueJSON("second")
	}()
	select {
	case err := <-result:
		if !errors.Is(err, errWSOutboundQueueFull) {
			t.Fatalf("queue full error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue-full enqueue blocked")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("queue-full connection was not closed")
	}
}

func TestWsSyncConnSynchronousQueueFullDoesNotBlock(t *testing.T) {
	c := &WsSyncConn{
		outbound: make(chan wsOutboundMessage, 1),
		done:     make(chan struct{}),
	}
	if err := c.EnqueueJSON("first"); err != nil {
		t.Fatalf("enqueue first message: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		result <- c.WriteJSON("second")
	}()
	select {
	case err := <-result:
		if !errors.Is(err, errWSOutboundQueueFull) {
			t.Fatalf("queue full error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue-full synchronous write blocked")
	}
}

func TestWsSyncConnCloseIsIdempotent(t *testing.T) {
	c := &WsSyncConn{done: make(chan struct{})}
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestWsSyncConnCloseReleasesSynchronousWrite(t *testing.T) {
	block := make(chan struct{})
	socket := &recordingWSOutboundSocket{
		entered: make(chan struct{}, 1),
		block:   block,
	}
	c := newTestWsSyncConn(socket, 1)
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- c.WriteJSON("blocked")
	}()

	select {
	case <-socket.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case err := <-writeDone:
		if !errors.Is(err, errWSConnectionClosed) {
			t.Fatalf("write error = %v, want connection closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("synchronous write remained blocked after close")
	}
	close(block)
}

func TestNewWsSyncConnUsesDefaultQueueSize(t *testing.T) {
	c := newWsSyncConn(nil, 0)
	defer c.Close()
	if got := cap(c.outbound); got != defaultWSOutboundQueueSize {
		t.Fatalf("queue capacity = %d, want %d", got, defaultWSOutboundQueueSize)
	}
}
