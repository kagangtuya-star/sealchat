package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"sealchat/service/perfprofiler"
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
		outbound:            make(chan wsOutboundMessage, queueSize),
		interactiveOutbound: make(chan wsOutboundMessage, wsInteractiveQueueSize),
		done:                make(chan struct{}),
		outboundSocket:      socket,
		coalesced:           make(map[string]wsCoalescedEntry),
		coalescedWake:       make(chan struct{}, 1),
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

func resetWSPerfProfiler(t *testing.T) *perfprofiler.Manager {
	t.Helper()
	m := perfprofiler.Init(perfprofiler.Config{Enabled: true})
	if err := m.ApplyConfig(perfprofiler.Config{Enabled: true}); err != nil {
		t.Fatalf("enable profiler: %v", err)
	}
	m.ResetMessagePipeline()
	return m
}

func TestWsSyncConnAsyncEnqueueUsesDataWriteTimeout(t *testing.T) {
	c := &WsSyncConn{
		outbound:            make(chan wsOutboundMessage, 1),
		interactiveOutbound: make(chan wsOutboundMessage, 1),
		done:                make(chan struct{}),
		coalesced:           make(map[string]wsCoalescedEntry),
		coalescedWake:       make(chan struct{}, 1),
	}
	defer c.Close()

	if err := c.enqueueInteractiveJSON(wsReliableClassOther, wsOutboundDiagnosticNone, "interactive"); err != nil {
		t.Fatal(err)
	}
	if message := <-c.interactiveOutbound; message.timeout != wsDataWriteTimeout {
		t.Fatalf("interactive timeout = %v, want %v", message.timeout, wsDataWriteTimeout)
	}

	if err := c.enqueueReliableJSON(wsReliableClassOther, "reliable"); err != nil {
		t.Fatal(err)
	}
	if message := <-c.outbound; message.timeout != wsDataWriteTimeout {
		t.Fatalf("reliable timeout = %v, want %v", message.timeout, wsDataWriteTimeout)
	}

	if err := c.EnqueueCoalescedJSON("notice", "coalesced"); err != nil {
		t.Fatal(err)
	}
	message, ok := c.popOldestCoalesced()
	if !ok {
		t.Fatal("coalesced message was not enqueued")
	}
	if message.timeout != wsDataWriteTimeout {
		t.Fatalf("coalesced timeout = %v, want %v", message.timeout, wsDataWriteTimeout)
	}
}

func TestWsSyncConnEnqueueJSONHasNoMessageCreateDiagnosticTag(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c := &WsSyncConn{outbound: make(chan wsOutboundMessage, 1), done: make(chan struct{})}
	defer c.Close()
	if err := c.EnqueueJSON("ordinary"); err != nil {
		t.Fatal(err)
	}
	message := <-c.outbound
	if message.diagnosticTag != wsOutboundDiagnosticNone || message.enqueuedAtNs != 0 {
		t.Fatalf("ordinary message was tagged: %#v", message)
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ReliableEnqueuedTotal != 1 || ws.ReliableOther != 1 {
		t.Fatalf("ordinary reliable classification = %#v", ws)
	}
	if len(c.interactiveOutbound) != 0 {
		t.Fatal("ordinary reliable message entered interactive queue")
	}
}

func TestWsSyncConnMessageCreateResponseReliableClassification(t *testing.T) {
	m := resetWSPerfProfiler(t)
	socket := &recordingWSOutboundSocket{wrote: make(chan struct{}, 1)}
	c := newTestWsSyncConn(socket, 1)
	defer c.Close()

	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ReliableEnqueuedTotal != 1 || ws.ReliableMessageCreateResponse != 1 {
		t.Fatalf("message.create response classification = %#v", ws)
	}
}

func TestWsSyncConnMessageCreateResponseCapturesEnqueueState(t *testing.T) {
	c := &WsSyncConn{interactiveOutbound: make(chan wsOutboundMessage, 3), done: make(chan struct{})}
	defer c.Close()
	if err := c.enqueueInteractiveJSON(wsReliableClassOther, wsOutboundDiagnosticNone, "ahead"); err != nil {
		t.Fatal(err)
	}
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	<-c.interactiveOutbound
	message := <-c.interactiveOutbound
	if message.enqueuedAtNs <= 0 || message.queueDepthAtEnqueue != 1 {
		t.Fatalf("unexpected response enqueue diagnostics: %#v", message)
	}
}

func TestWsSyncConnMessageCreateResponseQueueWaitTiming(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c, socket, release := blockedCoalescedTestConn(t, 2)
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	release()
	waitForWrites(t, socket.wrote, 2)
	if err := c.WriteJSON("barrier"); err != nil {
		t.Fatal(err)
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ResponseQueueWait.Count != 1 || ws.ResponseQueueWait.P50Ms <= 0 {
		t.Fatalf("queue wait was not recorded: %#v", ws.ResponseQueueWait)
	}
}

func TestWsSyncConnMessageCreateResponseSocketWriteTiming(t *testing.T) {
	m := resetWSPerfProfiler(t)
	block := make(chan struct{})
	socket := &recordingWSOutboundSocket{entered: make(chan struct{}, 1), wrote: make(chan struct{}, 1), block: block}
	c := newTestWsSyncConn(socket, 1)
	defer c.Close()
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	waitForWrites(t, socket.entered, 1)
	close(block)
	waitForWrites(t, socket.wrote, 1)
	if err := c.WriteJSON("barrier"); err != nil {
		t.Fatal(err)
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ResponseSocketWrite.Count != 1 || ws.ResponseSocketWrite.P50Ms <= 0 {
		t.Fatalf("socket write was not recorded: %#v", ws.ResponseSocketWrite)
	}
}

func TestWsSyncConnMessageResponseTimingWaitsForWriter(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c, socket, release := blockedCoalescedTestConn(t, 1)
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	if got := messageResponseWriteCount(m); got != 0 {
		t.Fatalf("response write timing recorded at enqueue: count=%d", got)
	}
	release()
	waitForWrites(t, socket.wrote, 2)
	if err := c.WriteJSON("barrier"); err != nil {
		t.Fatal(err)
	}
	if got := messageResponseWriteCount(m); got != 1 {
		t.Fatalf("response write timing count = %d, want 1", got)
	}
}

func messageResponseWriteCount(m *perfprofiler.Manager) int {
	for _, stage := range m.MessagePipelineSummary(10 * time.Second).Stages {
		if stage.Key == "api_response_write" {
			return stage.Count
		}
	}
	return 0
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

func TestWsSyncConnMessageCreateResponseQueueFullClosesAndCounts(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c := &WsSyncConn{interactiveOutbound: make(chan wsOutboundMessage, 1), done: make(chan struct{})}
	if err := c.enqueueInteractiveJSON(wsReliableClassOther, wsOutboundDiagnosticNone, "first"); err != nil {
		t.Fatal(err)
	}
	before := m.MessagePipelineSummary(10 * time.Second).WS.ReliableEnqueuedTotal
	started := time.Now()
	err := c.enqueueMessageCreateResponseJSON("response")
	if !errors.Is(err, errWSOutboundQueueFull) {
		t.Fatalf("queue full error = %v", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("queue-full interactive enqueue blocked")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("response queue full did not close connection")
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ReliableQueueFull != 1 || ws.ResponseQueueFull != 1 {
		t.Fatalf("queue full counters = %#v", ws)
	}
	if ws.ReliableEnqueuedTotal != before || ws.ReliableMessageCreateResponse != 0 {
		t.Fatalf("queue-full frame was classified as enqueued: %#v", ws)
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
	if got := cap(c.interactiveOutbound); got != wsInteractiveQueueSize {
		t.Fatalf("interactive queue capacity = %d, want %d", got, wsInteractiveQueueSize)
	}
}

func blockedCoalescedTestConn(t *testing.T, size int) (*WsSyncConn, *recordingWSOutboundSocket, func()) {
	t.Helper()
	block := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(block) }) }
	socket := &recordingWSOutboundSocket{entered: make(chan struct{}, 1), wrote: make(chan struct{}, 256), block: block}
	c := newTestWsSyncConn(socket, size)
	t.Cleanup(func() { _ = c.Close(); release() })
	if err := c.EnqueueJSON("blocked"); err != nil {
		t.Fatal(err)
	}
	waitForWrites(t, socket.entered, 1)
	return c, socket, release
}

func enqueueCoalescedTest(t *testing.T, c *WsSyncConn, key, value string) {
	t.Helper()
	if err := c.EnqueueCoalescedJSON(key, value); err != nil {
		t.Fatal(err)
	}
}

func TestWsSyncConnCoalescedLatestWins(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 1)
	for _, value := range []string{"notice-1", "notice-2", "notice-3"} {
		enqueueCoalescedTest(t, c, "notice", value)
	}
	if len(c.coalescedWake) != 1 {
		t.Fatal("wake signals must coalesce")
	}
	release()
	waitForWrites(t, socket.wrote, 2)
	if err := c.WriteJSON("barrier"); err != nil {
		t.Fatal(err)
	}
	if got := socket.decodedStrings(t); !reflect.DeepEqual(got, []string{"blocked", "notice-3", "barrier"}) {
		t.Fatalf("writes = %v", got)
	}
}

func TestWsSyncConnCoalescedDiagnosticCounters(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c := &WsSyncConn{
		outbound:      make(chan wsOutboundMessage, 1),
		done:          make(chan struct{}),
		coalesced:     make(map[string]wsCoalescedEntry),
		coalescedWake: make(chan struct{}, 1),
	}
	defer c.Close()
	enqueueCoalescedTest(t, c, "same", "first")
	enqueueCoalescedTest(t, c, "same", "second")
	for i := 0; i < maxWSCoalescedKeys; i++ {
		enqueueCoalescedTest(t, c, fmt.Sprintf("new-%d", i), "notice")
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.CoalescedEnqueued != maxWSCoalescedKeys+2 || ws.CoalescedReplaced != 1 || ws.CoalescedEvicted != 1 {
		t.Fatalf("coalesced counters = %#v", ws)
	}
	if ws.ReliableEnqueuedTotal != 0 {
		t.Fatalf("coalesced frames entered reliable classification: %#v", ws)
	}
}

func TestWsSyncConnCoalescedDifferentKeysSurvive(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 1)
	enqueueCoalescedTest(t, c, "a", "notice-a")
	enqueueCoalescedTest(t, c, "b", "notice-b")
	release()
	waitForWrites(t, socket.wrote, 3)
	if got := socket.decodedStrings(t); !reflect.DeepEqual(got, []string{"blocked", "notice-a", "notice-b"}) {
		t.Fatalf("writes = %v", got)
	}
}

func TestWsSyncConnCoalescedDoesNotConsumeReliableQueue(t *testing.T) {
	c, _, _ := blockedCoalescedTestConn(t, 2)
	for i := range maxWSCoalescedKeys {
		enqueueCoalescedTest(t, c, fmt.Sprint(i), "notice")
	}
	if len(c.outbound) != 0 {
		t.Fatal("notices consumed reliable queue")
	}
	if len(c.interactiveOutbound) != 0 {
		t.Fatal("notices consumed interactive queue")
	}
	for range cap(c.outbound) {
		if err := c.EnqueueJSON("reliable"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWsSyncConnInteractiveOvertakesQueuedReliable(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 3)
	for _, value := range []string{"A", "B"} {
		if err := c.EnqueueJSON(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}

	release()
	waitForWrites(t, socket.wrote, 4)
	if got := socket.decodedStrings(t); !reflect.DeepEqual(got, []string{"blocked", "response", "A", "B"}) {
		t.Fatalf("writes = %v", got)
	}
}

func TestWsSyncConnReliableFIFOUnchangedWithInteractive(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 3)
	for _, value := range []string{"A", "B", "C"} {
		if err := c.EnqueueJSON(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}

	release()
	waitForWrites(t, socket.wrote, 5)
	var reliable []string
	for _, value := range socket.decodedStrings(t) {
		if value == "A" || value == "B" || value == "C" {
			reliable = append(reliable, value)
		}
	}
	if !reflect.DeepEqual(reliable, []string{"A", "B", "C"}) {
		t.Fatalf("reliable order = %v", reliable)
	}
}

func TestWsSyncConnInteractiveDoesNotStarveReliable(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 1)
	for i := range wsInteractiveBurstBeforeReliable + 1 {
		if err := c.enqueueInteractiveJSON(wsReliableClassOther, wsOutboundDiagnosticNone, fmt.Sprintf("interactive-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.EnqueueJSON("reliable"); err != nil {
		t.Fatal(err)
	}

	release()
	waitForWrites(t, socket.wrote, wsInteractiveBurstBeforeReliable+3)
	got := socket.decodedStrings(t)
	if got[wsInteractiveBurstBeforeReliable+1] != "reliable" {
		t.Fatalf("reliable starved after interactive burst: %v", got)
	}
}

func TestWsSyncConnMessageCreateResponseEnqueueDoesNotWaitForSocket(t *testing.T) {
	m := resetWSPerfProfiler(t)
	c, _, _ := blockedCoalescedTestConn(t, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.enqueueMessageCreateResponseJSON("response")
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("message.create response enqueue waited for socket write")
	}
	ws := m.MessagePipelineSummary(10 * time.Second).WS
	if ws.ReliableMessageCreateResponse != 1 {
		t.Fatalf("message.create response classification = %#v", ws)
	}
}

func TestWsSyncConnCloseWithPendingInteractiveResponseDoesNotBlock(t *testing.T) {
	c, _, _ := blockedCoalescedTestConn(t, 1)
	if err := c.enqueueMessageCreateResponseJSON("response"); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close blocked with pending interactive response")
	}
}

func TestWsSyncConnCoalescedOverflowDoesNotCloseConnection(t *testing.T) {
	c, _, _ := blockedCoalescedTestConn(t, 1)
	for i := range maxWSCoalescedKeys + 10 {
		enqueueCoalescedTest(t, c, fmt.Sprint(i), "notice")
		c.coalescedMu.Lock()
		size := len(c.coalesced)
		c.coalescedMu.Unlock()
		if size > maxWSCoalescedKeys {
			t.Fatalf("map size = %d", size)
		}
	}
	c.coalescedMu.Lock()
	defer c.coalescedMu.Unlock()
	if len(c.coalesced) != maxWSCoalescedKeys {
		t.Fatal("unexpected map size")
	}
	for i := range maxWSCoalescedKeys + 10 {
		_, exists := c.coalesced[fmt.Sprint(i)]
		if exists != (i >= 10) {
			t.Fatalf("wrong eviction for %d", i)
		}
	}
	select {
	case <-c.done:
		t.Fatal("overflow closed connection")
	default:
	}
}

func TestWsSyncConnReliableFIFOUnchangedWithCoalesced(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 3)
	for _, value := range []string{"A", "B", "C"} {
		enqueueCoalescedTest(t, c, value, "notice-"+value)
		if err := c.EnqueueJSON(value); err != nil {
			t.Fatal(err)
		}
	}
	release()
	waitForWrites(t, socket.wrote, 7)
	var reliable []string
	for _, value := range socket.decodedStrings(t) {
		if value == "A" || value == "B" || value == "C" {
			reliable = append(reliable, value)
		}
	}
	if !reflect.DeepEqual(reliable, []string{"A", "B", "C"}) {
		t.Fatalf("reliable order = %v", reliable)
	}
}

func TestWsSyncConnReliablePreferredOverCoalesced(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, 1)
	for i := range maxWSCoalescedKeys {
		enqueueCoalescedTest(t, c, fmt.Sprint(i), "notice")
	}
	if err := c.EnqueueJSON("reliable"); err != nil {
		t.Fatal(err)
	}
	release()
	waitForWrites(t, socket.wrote, maxWSCoalescedKeys+2)
	if got := socket.decodedStrings(t); got[1] != "reliable" {
		t.Fatalf("reliable was delayed: %v", got)
	}
}

func TestWsSyncConnCoalescedNotStarved(t *testing.T) {
	c, socket, release := blockedCoalescedTestConn(t, wsReliableBurstBeforeCoalesced+2)
	for range wsReliableBurstBeforeCoalesced + 2 {
		if err := c.EnqueueJSON("reliable"); err != nil {
			t.Fatal(err)
		}
	}
	enqueueCoalescedTest(t, c, "notice", "notice")
	release()
	waitForWrites(t, socket.wrote, wsReliableBurstBeforeCoalesced+4)
	if got := socket.decodedStrings(t); got[wsReliableBurstBeforeCoalesced] != "notice" {
		t.Fatalf("notice starved: %v", got)
	}
}

func TestWsSyncConnCoalescedErrors(t *testing.T) {
	c, _, _ := blockedCoalescedTestConn(t, 1)
	if err := c.EnqueueCoalescedJSON(" \t", "notice"); err == nil {
		t.Fatal("empty key accepted")
	}
	if err := c.EnqueueCoalescedJSON("key", make(chan int)); err == nil {
		t.Fatal("marshal error ignored")
	}
	select {
	case <-c.done:
		t.Fatal("validation closed connection")
	default:
	}
	_ = c.Close()
	if err := c.EnqueueCoalescedJSON("key", "notice"); !errors.Is(err, errWSConnectionClosed) {
		t.Fatalf("closed enqueue = %v", err)
	}
}
