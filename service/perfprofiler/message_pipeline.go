package perfprofiler

import (
	"database/sql"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"sealchat/model"
)

const messageTimingBufferSize = 8192

type MessageCreateStage uint8

const (
	MessageStagePrepare MessageCreateStage = iota
	MessageStagePersist
	MessageStageChannelBroadcast
	MessageStageBotBroadcast
	MessageStageMemberRecent
	MessageStageChannelRecent
	MessageStageWebhook
	MessageStageMention
	MessageStageWorldNotice
)

type MessageCreateTiming struct {
	TimestampMs        int64
	PrepareNs          int64
	PersistNs          int64
	ChannelBroadcastNs int64
	BotBroadcastNs     int64
	MemberRecentNs     int64
	ChannelRecentNs    int64
	WebhookNs          int64
	MentionNs          int64
	WorldNoticeNs      int64
	TotalSolveNs       int64
}

type MessageResponseTiming struct {
	TimestampMs int64
	WriteNs     int64
}

type WSResponseTiming struct {
	TimestampMs     int64
	QueueWaitNs     int64
	SocketWriteNs   int64
	QueueDepthAhead int
	Error           bool
}

type DigestTiming struct {
	TimestampMs     int64
	TotalNs         int64
	VisitorUpsertNs int64
	SpeakerUpsertNs int64
	Error           bool
}

type MessageCreateTrace struct {
	collector *messagePipelineCollector
	startedNs int64
	timing    MessageCreateTiming
}

type DigestTrace struct {
	collector *messagePipelineCollector
	startedNs int64
	timing    DigestTiming
}

type timingRing[T any] struct {
	items []T
	next  int
	size  int
}

func newTimingRing[T any](capacity int) timingRing[T] {
	if capacity < 1 {
		capacity = 1
	}
	return timingRing[T]{items: make([]T, capacity)}
}

func (r *timingRing[T]) add(item T) {
	r.items[r.next] = item
	r.next = (r.next + 1) % len(r.items)
	if r.size < len(r.items) {
		r.size++
	}
}

func (r *timingRing[T]) snapshot() []T {
	items := make([]T, 0, r.size)
	start := (r.next - r.size + len(r.items)) % len(r.items)
	for i := 0; i < r.size; i++ {
		items = append(items, r.items[(start+i)%len(r.items)])
	}
	return items
}

func (r *timingRing[T]) reset() {
	var zero T
	for i := range r.items {
		r.items[i] = zero
	}
	r.next = 0
	r.size = 0
}

type messagePipelineCollector struct {
	mu          sync.Mutex
	messages    timingRing[MessageCreateTiming]
	responses   timingRing[MessageResponseTiming]
	digests     timingRing[DigestTiming]
	wsResponses timingRing[WSResponseTiming]

	digestInFlight atomic.Int64
	digestPeak     atomic.Int64
	digestStarted  atomic.Int64
	digestComplete atomic.Int64
	digestErrors   atomic.Int64

	wsReliableQueueFull        atomic.Int64
	wsMessageResponseQueueFull atomic.Int64
	wsReliableMessageCreated   atomic.Int64
	wsReliableMessageResponse  atomic.Int64
	wsReliableBotEvent         atomic.Int64
	wsReliableOther            atomic.Int64
	wsCoalescedEnqueued        atomic.Int64
	wsCoalescedReplaced        atomic.Int64
	wsCoalescedEvicted         atomic.Int64

	sinceResetAt int64
	dbBaseline   sql.DBStats
	dbBaselineOK bool
}

func newMessagePipelineCollector(capacity int) *messagePipelineCollector {
	now := time.Now().UnixMilli()
	stats, ok := model.DBPoolStats()
	return &messagePipelineCollector{
		messages:     newTimingRing[MessageCreateTiming](capacity),
		responses:    newTimingRing[MessageResponseTiming](capacity),
		digests:      newTimingRing[DigestTiming](capacity),
		wsResponses:  newTimingRing[WSResponseTiming](capacity),
		sinceResetAt: now,
		dbBaseline:   stats,
		dbBaselineOK: ok,
	}
}

func BeginMessageCreateTrace() *MessageCreateTrace {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return nil
	}
	return &MessageCreateTrace{collector: m.pipeline, startedNs: time.Now().UnixNano()}
}

func (t *MessageCreateTrace) StageStart() int64 {
	if t == nil {
		return 0
	}
	return time.Now().UnixNano()
}

func (t *MessageCreateTrace) StageDone(stage MessageCreateStage, started int64) {
	if t == nil || started <= 0 {
		return
	}
	duration := time.Now().UnixNano() - started
	if duration <= 0 {
		return
	}
	switch stage {
	case MessageStagePrepare:
		t.timing.PrepareNs += duration
	case MessageStagePersist:
		t.timing.PersistNs += duration
	case MessageStageChannelBroadcast:
		t.timing.ChannelBroadcastNs += duration
	case MessageStageBotBroadcast:
		t.timing.BotBroadcastNs += duration
	case MessageStageMemberRecent:
		t.timing.MemberRecentNs += duration
	case MessageStageChannelRecent:
		t.timing.ChannelRecentNs += duration
	case MessageStageWebhook:
		t.timing.WebhookNs += duration
	case MessageStageMention:
		t.timing.MentionNs += duration
	case MessageStageWorldNotice:
		t.timing.WorldNoticeNs += duration
	}
}

func (t *MessageCreateTrace) Finish() {
	if t == nil || t.collector == nil {
		return
	}
	now := time.Now()
	t.timing.TimestampMs = now.UnixMilli()
	t.timing.TotalSolveNs = now.UnixNano() - t.startedNs
	t.collector.mu.Lock()
	t.collector.messages.add(t.timing)
	t.collector.mu.Unlock()
}

func RecordMessageResponseWrite(started int64) {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil || started <= 0 {
		return
	}
	duration := messageResponseWriteNowUnixNano() - started
	if duration < 0 {
		return
	}
	m.pipeline.mu.Lock()
	m.pipeline.responses.add(MessageResponseTiming{TimestampMs: time.Now().UnixMilli(), WriteNs: duration})
	m.pipeline.mu.Unlock()
}

var messageResponseWriteNowUnixNano = func() int64 {
	return time.Now().UnixNano()
}

func BeginMessageResponseWrite() int64 {
	m := defaultManager
	if m == nil || !m.enabled.Load() {
		return 0
	}
	return time.Now().UnixNano()
}

func RecordMessageResponseOutbound(queueWaitNs, socketWriteNs int64, queueDepthAhead int, writeError bool) {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	m.pipeline.mu.Lock()
	m.pipeline.wsResponses.add(WSResponseTiming{
		TimestampMs:     time.Now().UnixMilli(),
		QueueWaitNs:     queueWaitNs,
		SocketWriteNs:   socketWriteNs,
		QueueDepthAhead: queueDepthAhead,
		Error:           writeError,
	})
	m.pipeline.mu.Unlock()
}

func RecordWSReliableQueueFull(messageCreateResponse bool) {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	m.pipeline.wsReliableQueueFull.Add(1)
	if messageCreateResponse {
		m.pipeline.wsMessageResponseQueueFull.Add(1)
	}
}

const (
	wsReliableClassOther uint8 = iota
	wsReliableClassMessageCreated
	wsReliableClassMessageCreateResponse
	wsReliableClassBotEvent
)

func RecordWSReliableEnqueued(class uint8) {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	switch class {
	case wsReliableClassMessageCreated:
		m.pipeline.wsReliableMessageCreated.Add(1)
	case wsReliableClassMessageCreateResponse:
		m.pipeline.wsReliableMessageResponse.Add(1)
	case wsReliableClassBotEvent:
		m.pipeline.wsReliableBotEvent.Add(1)
	default:
		m.pipeline.wsReliableOther.Add(1)
	}
}

func RecordWSCoalescedEnqueued() {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	m.pipeline.wsCoalescedEnqueued.Add(1)
}

func RecordWSCoalescedReplaced() {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	m.pipeline.wsCoalescedReplaced.Add(1)
}

func RecordWSCoalescedEvicted() {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return
	}
	m.pipeline.wsCoalescedEvicted.Add(1)
}

func BeginDigestTrace() *DigestTrace {
	m := defaultManager
	if m == nil || !m.enabled.Load() || m.pipeline == nil {
		return nil
	}
	c := m.pipeline
	c.digestStarted.Add(1)
	inFlight := c.digestInFlight.Add(1)
	for {
		peak := c.digestPeak.Load()
		if inFlight <= peak || c.digestPeak.CompareAndSwap(peak, inFlight) {
			break
		}
	}
	return &DigestTrace{collector: c, startedNs: time.Now().UnixNano()}
}

func (t *DigestTrace) StageStart() int64 {
	if t == nil {
		return 0
	}
	return time.Now().UnixNano()
}

func (t *DigestTrace) VisitorUpsertDone(started int64) {
	if t != nil && started > 0 {
		if duration := time.Now().UnixNano() - started; duration > 0 {
			t.timing.VisitorUpsertNs += duration
		}
	}
}

func (t *DigestTrace) SpeakerUpsertDone(started int64) {
	if t != nil && started > 0 {
		if duration := time.Now().UnixNano() - started; duration > 0 {
			t.timing.SpeakerUpsertNs += duration
		}
	}
}

func (t *DigestTrace) Finish(result *error) {
	if t == nil || t.collector == nil {
		return
	}
	now := time.Now()
	t.timing.TimestampMs = now.UnixMilli()
	t.timing.TotalNs = now.UnixNano() - t.startedNs
	t.timing.Error = result != nil && *result != nil
	t.collector.digestInFlight.Add(-1)
	t.collector.digestComplete.Add(1)
	if t.timing.Error {
		t.collector.digestErrors.Add(1)
	}
	t.collector.mu.Lock()
	t.collector.digests.add(t.timing)
	t.collector.mu.Unlock()
}

func (m *Manager) MessagePipelineSummary(window time.Duration) MessagePipelineSummary {
	if window < 10*time.Second {
		window = 10 * time.Second
	}
	if window > 10*time.Minute {
		window = 10 * time.Minute
	}
	now := time.Now()
	cutoff := now.Add(-window).UnixMilli()
	c := m.pipeline
	if c == nil {
		return MessagePipelineSummary{UpdatedAt: now.UnixMilli(), WindowSec: int(window / time.Second)}
	}
	c.mu.Lock()
	messages := c.messages.snapshot()
	responses := c.responses.snapshot()
	digests := c.digests.snapshot()
	wsResponses := c.wsResponses.snapshot()
	sinceResetAt := c.sinceResetAt
	baseline := c.dbBaseline
	baselineOK := c.dbBaselineOK
	c.mu.Unlock()

	stageDefs := []struct {
		key, label string
		value      func(MessageCreateTiming) int64
	}{
		{"prepare", "Prepare", func(v MessageCreateTiming) int64 { return v.PrepareNs }},
		{"persist", "Message Persist", func(v MessageCreateTiming) int64 { return v.PersistNs }},
		{"channel_broadcast", "Channel Broadcast", func(v MessageCreateTiming) int64 { return v.ChannelBroadcastNs }},
		{"bot_broadcast", "BOT Broadcast", func(v MessageCreateTiming) int64 { return v.BotBroadcastNs }},
		{"member_recent", "Member Recent", func(v MessageCreateTiming) int64 { return v.MemberRecentNs }},
		{"channel_recent", "Channel Recent", func(v MessageCreateTiming) int64 { return v.ChannelRecentNs }},
		{"webhook", "Webhook Event Log", func(v MessageCreateTiming) int64 { return v.WebhookNs }},
		{"mention", "Mention", func(v MessageCreateTiming) int64 { return v.MentionNs }},
		{"world_notice", "World Notice", func(v MessageCreateTiming) int64 { return v.WorldNoticeNs }},
		{"total_solve", "Total Solve", func(v MessageCreateTiming) int64 { return v.TotalSolveNs }},
	}
	filteredMessages := make([]MessageCreateTiming, 0, len(messages))
	for _, item := range messages {
		if item.TimestampMs >= cutoff {
			filteredMessages = append(filteredMessages, item)
		}
	}
	stages := make([]MessagePipelineStage, 0, len(stageDefs)+1)
	for _, def := range stageDefs {
		values := make([]int64, 0, len(filteredMessages))
		for _, item := range filteredMessages {
			if value := def.value(item); value > 0 {
				values = append(values, value)
			}
		}
		stages = append(stages, MessagePipelineStage{Key: def.key, Label: def.label, DurationStats: durationStats(values)})
	}
	responseValues := make([]int64, 0, len(responses))
	for _, item := range responses {
		if item.TimestampMs >= cutoff && item.WriteNs >= 0 {
			responseValues = append(responseValues, item.WriteNs)
		}
	}
	stages = append(stages, MessagePipelineStage{Key: "api_response_write", Label: "API Response Write", DurationStats: durationStatsNonNegative(responseValues)})

	digestTotal := make([]int64, 0, len(digests))
	digestVisitor := make([]int64, 0, len(digests))
	digestSpeaker := make([]int64, 0, len(digests))
	for _, item := range digests {
		if item.TimestampMs < cutoff {
			continue
		}
		if item.TotalNs > 0 {
			digestTotal = append(digestTotal, item.TotalNs)
		}
		if item.VisitorUpsertNs > 0 {
			digestVisitor = append(digestVisitor, item.VisitorUpsertNs)
		}
		if item.SpeakerUpsertNs > 0 {
			digestSpeaker = append(digestSpeaker, item.SpeakerUpsertNs)
		}
	}

	wsQueueWait := make([]int64, 0, len(wsResponses))
	wsSocketWrite := make([]int64, 0, len(wsResponses))
	wsQueueDepth := make([]int, 0, len(wsResponses))
	var wsResponseErrors int64
	for _, item := range wsResponses {
		if item.TimestampMs < cutoff {
			continue
		}
		if item.QueueWaitNs >= 0 {
			wsQueueWait = append(wsQueueWait, item.QueueWaitNs)
		}
		if item.SocketWriteNs >= 0 {
			wsSocketWrite = append(wsSocketWrite, item.SocketWriteNs)
		}
		wsQueueDepth = append(wsQueueDepth, item.QueueDepthAhead)
		if item.Error {
			wsResponseErrors++
		}
	}

	reliableMessageCreated := c.wsReliableMessageCreated.Load()
	reliableMessageResponse := c.wsReliableMessageResponse.Load()
	reliableBotEvent := c.wsReliableBotEvent.Load()
	reliableOther := c.wsReliableOther.Load()
	summary := MessagePipelineSummary{
		UpdatedAt:    now.UnixMilli(),
		SinceResetAt: sinceResetAt,
		WindowSec:    int(window / time.Second),
		MessageCount: len(filteredMessages),
		Stages:       stages,
		Digest: DigestStats{
			Started:        c.digestStarted.Load(),
			Completed:      c.digestComplete.Load(),
			Errors:         c.digestErrors.Load(),
			InFlight:       c.digestInFlight.Load(),
			PeakInFlight:   c.digestPeak.Load(),
			Total:          durationStats(digestTotal),
			VisitorUpserts: durationStats(digestVisitor),
			SpeakerUpserts: durationStats(digestSpeaker),
		},
		WS: WSOutboundStats{
			ResponseQueueWait:             durationStatsNonNegative(wsQueueWait),
			ResponseSocketWrite:           durationStatsNonNegative(wsSocketWrite),
			ResponseQueueDepth:            intStats(wsQueueDepth),
			ResponseErrors:                wsResponseErrors,
			ReliableQueueFull:             c.wsReliableQueueFull.Load(),
			ResponseQueueFull:             c.wsMessageResponseQueueFull.Load(),
			ReliableEnqueuedTotal:         reliableMessageCreated + reliableMessageResponse + reliableBotEvent + reliableOther,
			ReliableMessageCreated:        reliableMessageCreated,
			ReliableMessageCreateResponse: reliableMessageResponse,
			ReliableBotEvent:              reliableBotEvent,
			ReliableOther:                 reliableOther,
			CoalescedEnqueued:             c.wsCoalescedEnqueued.Load(),
			CoalescedReplaced:             c.wsCoalescedReplaced.Load(),
			CoalescedEvicted:              c.wsCoalescedEvicted.Load(),
		},
	}
	if stats, ok := model.DBPoolStats(); ok {
		summary.DB = buildDBStats(stats, baseline, baselineOK)
	}
	return summary
}

func (m *Manager) ResetMessagePipeline() {
	if m == nil || m.pipeline == nil {
		return
	}
	c := m.pipeline
	stats, ok := model.DBPoolStats()
	c.mu.Lock()
	c.messages.reset()
	c.responses.reset()
	c.digests.reset()
	c.wsResponses.reset()
	c.sinceResetAt = time.Now().UnixMilli()
	c.dbBaseline = stats
	c.dbBaselineOK = ok
	c.mu.Unlock()
	c.digestStarted.Store(0)
	c.digestComplete.Store(0)
	c.digestErrors.Store(0)
	c.digestPeak.Store(0)
	c.wsReliableQueueFull.Store(0)
	c.wsMessageResponseQueueFull.Store(0)
	c.wsReliableMessageCreated.Store(0)
	c.wsReliableMessageResponse.Store(0)
	c.wsReliableBotEvent.Store(0)
	c.wsReliableOther.Store(0)
	c.wsCoalescedEnqueued.Store(0)
	c.wsCoalescedReplaced.Store(0)
	c.wsCoalescedEvicted.Store(0)
}

func durationStats(values []int64) DurationStats {
	positive := values[:0]
	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}
	if len(positive) == 0 {
		return DurationStats{}
	}
	sort.Slice(positive, func(i, j int) bool { return positive[i] < positive[j] })
	toMs := func(value int64) float64 { return float64(value) / float64(time.Millisecond) }
	nearestRank := func(percentile float64) int64 {
		index := int(math.Ceil(percentile*float64(len(positive)))) - 1
		if index < 0 {
			index = 0
		}
		return positive[index]
	}
	return DurationStats{
		Count: len(positive),
		P50Ms: toMs(nearestRank(0.50)),
		P95Ms: toMs(nearestRank(0.95)),
		P99Ms: toMs(nearestRank(0.99)),
		MaxMs: toMs(positive[len(positive)-1]),
	}
}

func durationStatsNonNegative(values []int64) DurationStats {
	nonNegative := values[:0]
	for _, value := range values {
		if value >= 0 {
			nonNegative = append(nonNegative, value)
		}
	}
	if len(nonNegative) == 0 {
		return DurationStats{}
	}
	sort.Slice(nonNegative, func(i, j int) bool { return nonNegative[i] < nonNegative[j] })
	toMs := func(value int64) float64 { return float64(value) / float64(time.Millisecond) }
	nearestRank := func(percentile float64) int64 {
		index := int(math.Ceil(percentile*float64(len(nonNegative)))) - 1
		if index < 0 {
			index = 0
		}
		return nonNegative[index]
	}
	return DurationStats{
		Count: len(nonNegative),
		P50Ms: toMs(nearestRank(0.50)),
		P95Ms: toMs(nearestRank(0.95)),
		P99Ms: toMs(nearestRank(0.99)),
		MaxMs: toMs(nonNegative[len(nonNegative)-1]),
	}
}

func intStats(values []int) IntStats {
	nonNegative := values[:0]
	for _, value := range values {
		if value >= 0 {
			nonNegative = append(nonNegative, value)
		}
	}
	if len(nonNegative) == 0 {
		return IntStats{}
	}
	sort.Ints(nonNegative)
	nearestRank := func(percentile float64) int {
		index := int(math.Ceil(percentile*float64(len(nonNegative)))) - 1
		if index < 0 {
			index = 0
		}
		return nonNegative[index]
	}
	return IntStats{
		Count: len(nonNegative),
		P50:   nearestRank(0.50),
		P95:   nearestRank(0.95),
		P99:   nearestRank(0.99),
		Max:   nonNegative[len(nonNegative)-1],
	}
}

func buildDBStats(stats, baseline sql.DBStats, baselineOK bool) DBStats {
	waitCountDelta := stats.WaitCount
	waitDurationDelta := stats.WaitDuration
	if baselineOK {
		waitCountDelta -= baseline.WaitCount
		waitDurationDelta -= baseline.WaitDuration
		if waitCountDelta < 0 {
			waitCountDelta = 0
		}
		if waitDurationDelta < 0 {
			waitDurationDelta = 0
		}
	}
	return DBStats{
		Driver:              model.DBDriver(),
		MaxOpenConnections:  stats.MaxOpenConnections,
		OpenConnections:     stats.OpenConnections,
		InUse:               stats.InUse,
		Idle:                stats.Idle,
		WaitCount:           stats.WaitCount,
		WaitDurationMs:      float64(stats.WaitDuration) / float64(time.Millisecond),
		WaitCountDelta:      waitCountDelta,
		WaitDurationMsDelta: float64(waitDurationDelta) / float64(time.Millisecond),
		MaxIdleClosed:       stats.MaxIdleClosed,
		MaxIdleTimeClosed:   stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:   stats.MaxLifetimeClosed,
	}
}
