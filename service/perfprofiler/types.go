package perfprofiler

import "time"

type Config struct {
	Enabled             bool
	OutputDir           string
	LightSampleInterval time.Duration
	SnapshotInterval    time.Duration
	CPUProfileDuration  time.Duration
	Retention           time.Duration
}

type SamplePoint struct {
	Timestamp      int64  `json:"timestamp"`
	Goroutines     int    `json:"goroutines"`
	HeapAllocBytes uint64 `json:"heapAllocBytes"`
	HeapInuseBytes uint64 `json:"heapInuseBytes"`
	HeapSysBytes   uint64 `json:"heapSysBytes"`
	StackInuse     uint64 `json:"stackInuseBytes"`
	GCCycles       uint32 `json:"gcCycles"`
	LastPauseNs    uint64 `json:"lastPauseNs"`
}

type CPUSessionState struct {
	SessionID   string `json:"sessionId"`
	Active      bool   `json:"active"`
	StartedAt   int64  `json:"startedAt"`
	EndsAt      int64  `json:"endsAt"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	LastError   string `json:"lastError,omitempty"`
	AutoStopped bool   `json:"autoStopped"`
}

type TraceSessionState struct {
	SessionID   string `json:"sessionId"`
	Active      bool   `json:"active"`
	StartedAt   int64  `json:"startedAt"`
	EndsAt      int64  `json:"endsAt"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	LastError   string `json:"lastError,omitempty"`
	AutoStopped bool   `json:"autoStopped"`
}

type State struct {
	Enabled               bool               `json:"enabled"`
	Status                string             `json:"status"`
	OutputDir             string             `json:"outputDir"`
	LightIntervalSec      int                `json:"lightIntervalSec"`
	SnapshotIntervalSec   int                `json:"snapshotIntervalSec"`
	CPUProfileDurationSec int                `json:"cpuProfileDurationSec"`
	RetentionDays         int                `json:"retentionDays"`
	LastSampleAt          int64              `json:"lastSampleAt"`
	LastError             string             `json:"lastError,omitempty"`
	Latest                *SamplePoint       `json:"latest,omitempty"`
	CPUSession            *CPUSessionState   `json:"cpuSession,omitempty"`
	TraceSession          *TraceSessionState `json:"traceSession,omitempty"`
}

type DurationStats struct {
	Count int     `json:"count"`
	P50Ms float64 `json:"p50Ms"`
	P95Ms float64 `json:"p95Ms"`
	P99Ms float64 `json:"p99Ms"`
	MaxMs float64 `json:"maxMs"`
}

type IntStats struct {
	Count int `json:"count"`
	P50   int `json:"p50"`
	P95   int `json:"p95"`
	P99   int `json:"p99"`
	Max   int `json:"max"`
}

type MessagePipelineStage struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	DurationStats
}

type DigestStats struct {
	Started        int64         `json:"started"`
	Completed      int64         `json:"completed"`
	Errors         int64         `json:"errors"`
	InFlight       int64         `json:"inFlight"`
	PeakInFlight   int64         `json:"peakInFlight"`
	Total          DurationStats `json:"total"`
	VisitorUpserts DurationStats `json:"visitorUpserts"`
	SpeakerUpserts DurationStats `json:"speakerUpserts"`
}

type DBStats struct {
	Driver              string  `json:"driver"`
	MaxOpenConnections  int     `json:"maxOpenConnections"`
	OpenConnections     int     `json:"openConnections"`
	InUse               int     `json:"inUse"`
	Idle                int     `json:"idle"`
	WaitCount           int64   `json:"waitCount"`
	WaitDurationMs      float64 `json:"waitDurationMs"`
	WaitCountDelta      int64   `json:"waitCountDelta"`
	WaitDurationMsDelta float64 `json:"waitDurationMsDelta"`
	MaxIdleClosed       int64   `json:"maxIdleClosed"`
	MaxIdleTimeClosed   int64   `json:"maxIdleTimeClosed"`
	MaxLifetimeClosed   int64   `json:"maxLifetimeClosed"`
}

type WSOutboundStats struct {
	ResponseQueueWait   DurationStats `json:"responseQueueWait"`
	ResponseSocketWrite DurationStats `json:"responseSocketWrite"`
	ResponseQueueDepth  IntStats      `json:"responseQueueDepth"`

	ResponseErrors                int64 `json:"responseErrors"`
	ReliableQueueFull             int64 `json:"reliableQueueFull"`
	ResponseQueueFull             int64 `json:"responseQueueFull"`
	ReliableEnqueuedTotal         int64 `json:"reliableEnqueuedTotal"`
	ReliableMessageCreated        int64 `json:"reliableMessageCreated"`
	ReliableMessageCreateResponse int64 `json:"reliableMessageCreateResponse"`
	ReliableBotEvent              int64 `json:"reliableBotEvent"`
	ReliableOther                 int64 `json:"reliableOther"`

	CoalescedEnqueued int64 `json:"coalescedEnqueued"`
	CoalescedReplaced int64 `json:"coalescedReplaced"`
	CoalescedEvicted  int64 `json:"coalescedEvicted"`
}

type MessagePipelineSummary struct {
	UpdatedAt    int64                  `json:"updatedAt"`
	SinceResetAt int64                  `json:"sinceResetAt"`
	WindowSec    int                    `json:"windowSec"`
	MessageCount int                    `json:"messageCount"`
	Stages       []MessagePipelineStage `json:"stages"`
	Digest       DigestStats            `json:"digest"`
	DB           DBStats                `json:"db"`
	WS           WSOutboundStats        `json:"ws"`
}

type Artifact struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
	Path       string `json:"-"`
}

type TopFunction struct {
	Name       string `json:"name"`
	Flat       int64  `json:"flat"`
	Cumulative int64  `json:"cumulative"`
	FlatPct    string `json:"flatPct"`
	CumPct     string `json:"cumPct"`
	Source     string `json:"source"`
	Unit       string `json:"unit"`
}
