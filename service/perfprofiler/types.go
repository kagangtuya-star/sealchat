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

type State struct {
	Enabled               bool             `json:"enabled"`
	Status                string           `json:"status"`
	OutputDir             string           `json:"outputDir"`
	LightIntervalSec      int              `json:"lightIntervalSec"`
	SnapshotIntervalSec   int              `json:"snapshotIntervalSec"`
	CPUProfileDurationSec int              `json:"cpuProfileDurationSec"`
	RetentionDays         int              `json:"retentionDays"`
	LastSampleAt          int64            `json:"lastSampleAt"`
	LastError             string           `json:"lastError,omitempty"`
	Latest                *SamplePoint     `json:"latest,omitempty"`
	CPUSession            *CPUSessionState `json:"cpuSession,omitempty"`
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
