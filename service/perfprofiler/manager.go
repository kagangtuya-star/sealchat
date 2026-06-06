package perfprofiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	pprofprofile "github.com/google/pprof/profile"

	"sealchat/utils"
)

var (
	defaultManager *Manager
	initOnce       sync.Once

	ErrDisabled      = errors.New("performance profiler disabled")
	ErrSessionActive = errors.New("cpu session already active")
)

type Manager struct {
	mu            sync.RWMutex
	cfg           Config
	state         State
	rootCtx       context.Context
	cancel        context.CancelFunc
	started       bool
	sessionCancel context.CancelFunc
	wg            sync.WaitGroup
}

func ConfigFromApp(cfg utils.PerformanceProfilerConfig) Config {
	retention := time.Duration(cfg.RetentionDays) * 24 * time.Hour
	return Config{
		Enabled:             cfg.Enabled,
		OutputDir:           cfg.OutputDir,
		LightSampleInterval: time.Duration(cfg.LightSampleIntervalSec) * time.Second,
		SnapshotInterval:    time.Duration(cfg.SnapshotIntervalSec) * time.Second,
		CPUProfileDuration:  time.Duration(cfg.CPUProfileDurationSec) * time.Second,
		Retention:           retention,
	}
}

func Init(cfg Config) *Manager {
	initOnce.Do(func() {
		defaultManager = newManager(cfg)
	})
	if defaultManager != nil {
		_ = defaultManager.ApplyConfig(cfg)
	}
	return defaultManager
}

func Get() *Manager {
	return defaultManager
}

func newManager(cfg Config) *Manager {
	mgr := &Manager{}
	_ = mgr.ApplyConfig(cfg)
	return mgr
}

func (m *Manager) ApplyConfig(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	normalizeConfig(&cfg)
	m.cfg = cfg
	m.state.Enabled = cfg.Enabled
	m.state.OutputDir = cfg.OutputDir
	m.state.LightIntervalSec = int(cfg.LightSampleInterval / time.Second)
	m.state.SnapshotIntervalSec = int(cfg.SnapshotInterval / time.Second)
	m.state.CPUProfileDurationSec = int(cfg.CPUProfileDuration / time.Second)
	m.state.RetentionDays = int(cfg.Retention / (24 * time.Hour))
	if !cfg.Enabled {
		m.state.Status = "disabled"
	} else if m.state.Status == "" || m.state.Status == "disabled" {
		m.state.Status = "idle"
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if ctx == nil {
		ctx = context.Background()
	}
	m.rootCtx = ctx
	if m.started {
		m.mu.Unlock()
		return
	}
	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.started = true
	enabled := m.cfg.Enabled
	m.mu.Unlock()

	if !enabled {
		return
	}
	m.wg.Add(1)
	go m.loop(childCtx)
}

func (m *Manager) Reconfigure(cfg Config) error {
	if m == nil {
		return nil
	}
	m.Stop()
	if err := m.ApplyConfig(cfg); err != nil {
		return err
	}
	m.mu.RLock()
	rootCtx := m.rootCtx
	m.mu.RUnlock()
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	m.Start(rootCtx)
	return nil
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	if m.sessionCancel != nil {
		m.sessionCancel()
	}
	m.started = false
	m.cancel = nil
	m.sessionCancel = nil
	if m.cfg.Enabled {
		m.state.Status = "idle"
	} else {
		m.state.Status = "disabled"
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) loop(ctx context.Context) {
	defer m.wg.Done()
	m.sampleOnce()
	lightTicker := time.NewTicker(m.cfg.LightSampleInterval)
	defer lightTicker.Stop()
	snapshotTicker := time.NewTicker(m.cfg.SnapshotInterval)
	defer snapshotTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-lightTicker.C:
			m.sampleOnce()
			_ = cleanupExpiredFiles(m.cfg.OutputDir, m.cfg.Retention)
		case <-snapshotTicker.C:
			_ = m.captureSnapshot()
		}
	}
}

func (m *Manager) sampleOnce() {
	point := collectSample()
	now := point.Timestamp
	currentPath := filepath.Join(m.cfg.OutputDir, "current.json")
	samplesPath := filepath.Join(m.cfg.OutputDir, fmt.Sprintf("samples-%s.jsonl", time.UnixMilli(now).Format("2006-01-02")))
	_ = writeJSONFile(currentPath, point)
	_ = appendJSONL(samplesPath, point)

	m.mu.Lock()
	defer m.mu.Unlock()
	copyPoint := point
	m.state.Latest = &copyPoint
	m.state.LastSampleAt = now
	if m.state.Enabled {
		m.state.Status = "running"
	}
	_ = writeJSONFile(filepath.Join(m.cfg.OutputDir, "current-state.json"), m.state)
}

func collectSample() SamplePoint {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	lastPause := uint64(0)
	if mem.NumGC > 0 {
		lastPause = mem.PauseNs[(mem.NumGC+255)%256]
	}
	return SamplePoint{
		Timestamp:      time.Now().UnixMilli(),
		Goroutines:     runtime.NumGoroutine(),
		HeapAllocBytes: mem.HeapAlloc,
		HeapInuseBytes: mem.HeapInuse,
		HeapSysBytes:   mem.HeapSys,
		StackInuse:     mem.StackInuse,
		GCCycles:       mem.NumGC,
		LastPauseNs:    lastPause,
	}
}

func (m *Manager) CurrentState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.state
	if state.Latest != nil {
		copyPoint := *state.Latest
		state.Latest = &copyPoint
	}
	if state.CPUSession != nil {
		copySession := *state.CPUSession
		state.CPUSession = &copySession
	}
	return state
}

func (m *Manager) QueryHistory(startMs, endMs int64) ([]SamplePoint, error) {
	files, err := filepath.Glob(filepath.Join(m.cfg.OutputDir, "samples-*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	points := make([]SamplePoint, 0)
	for _, file := range files {
		var chunk []SamplePoint
		if err := readJSONL(file, &chunk); err != nil {
			return nil, err
		}
		for _, point := range chunk {
			if startMs > 0 && point.Timestamp < startMs {
				continue
			}
			if endMs > 0 && point.Timestamp > endMs {
				continue
			}
			points = append(points, point)
		}
	}
	return points, nil
}

func (m *Manager) ListArtifacts() ([]Artifact, error) {
	return listArtifacts(m.cfg.OutputDir)
}

func (m *Manager) ResolveArtifact(name string) (Artifact, error) {
	items, err := m.ListArtifacts()
	if err != nil {
		return Artifact{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return Artifact{}, os.ErrNotExist
}

func (m *Manager) TopFunctions(limit int) ([]TopFunction, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := m.ListArtifacts()
	if err != nil {
		return nil, err
	}
	activeSessionFile := ""
	m.mu.RLock()
	if m.state.CPUSession != nil && m.state.CPUSession.Active {
		activeSessionFile = m.state.CPUSession.FileName
	}
	m.mu.RUnlock()

	for i := range items {
		item := items[i]
		if item.Kind != "session-cpu" && item.Kind != "snapshot-cpu" {
			continue
		}
		if activeSessionFile != "" && item.Kind == "session-cpu" && item.Name == activeSessionFile {
			continue
		}
		rows, err := topFunctionsFromProfile(item.Path, item.Name, limit)
		if err != nil {
			continue
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return []TopFunction{}, nil
}

func (m *Manager) StartCPUSession(duration time.Duration) (*CPUSessionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled {
		return nil, ErrDisabled
	}
	if m.state.CPUSession != nil && m.state.CPUSession.Active {
		return nil, ErrSessionActive
	}
	if duration <= 0 {
		duration = m.cfg.CPUProfileDuration
	}
	if err := ensureDir(filepath.Join(m.cfg.OutputDir, "sessions")); err != nil {
		return nil, err
	}
	sessionID := time.Now().Format("20060102-150405.000")
	fileName := fmt.Sprintf("cpu-session-%s.pb.gz", sessionID)
	filePath := filepath.Join(m.cfg.OutputDir, "sessions", fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &CPUSessionState{
		SessionID: sessionID,
		Active:    true,
		StartedAt: time.Now().UnixMilli(),
		EndsAt:    time.Now().Add(duration).UnixMilli(),
		FileName:  fileName,
	}
	m.state.CPUSession = state
	m.sessionCancel = cancel
	m.wg.Add(1)
	go m.finishCPUSession(ctx, file, filePath, sessionID, duration)
	return cloneSessionState(state), nil
}

func (m *Manager) StopCPUSession() error {
	m.mu.Lock()
	cancel := m.sessionCancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (m *Manager) finishCPUSession(ctx context.Context, file *os.File, filePath, sessionID string, duration time.Duration) {
	defer m.wg.Done()
	autoStopped := true
	select {
	case <-ctx.Done():
		autoStopped = false
	case <-time.After(duration):
	}
	pprof.StopCPUProfile()
	_ = file.Close()
	info, _ := os.Stat(filePath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	meta := map[string]any{
		"sessionId":   sessionID,
		"fileName":    filepath.Base(filePath),
		"fileSize":    size,
		"autoStopped": autoStopped,
		"finishedAt":  time.Now().UnixMilli(),
	}
	_ = writeJSONFile(filepath.Join(m.cfg.OutputDir, "sessions", fmt.Sprintf("session-%s.json", sessionID)), meta)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.CPUSession != nil && m.state.CPUSession.SessionID == sessionID {
		m.state.CPUSession.Active = false
		m.state.CPUSession.FileSize = size
		m.state.CPUSession.AutoStopped = autoStopped
	}
	m.sessionCancel = nil
}

func (m *Manager) captureSnapshot() error {
	if !m.cfg.Enabled {
		return nil
	}
	snapshotDir := filepath.Join(m.cfg.OutputDir, "snapshots")
	if err := ensureDir(snapshotDir); err != nil {
		return err
	}
	ts := time.Now().Format("20060102-150405")
	heapFile, err := os.Create(filepath.Join(snapshotDir, fmt.Sprintf("heap-%s.pb.gz", ts)))
	if err != nil {
		return err
	}
	defer heapFile.Close()
	if err := pprof.Lookup("heap").WriteTo(heapFile, 0); err != nil {
		return err
	}
	m.mu.RLock()
	sessionActive := m.state.CPUSession != nil && m.state.CPUSession.Active
	m.mu.RUnlock()
	if !sessionActive {
		cpuFile, err := os.Create(filepath.Join(snapshotDir, fmt.Sprintf("cpu-%s.pb.gz", ts)))
		if err != nil {
			return err
		}
		if err := pprof.StartCPUProfile(cpuFile); err != nil {
			_ = cpuFile.Close()
			return err
		}
		time.Sleep(3 * time.Second)
		pprof.StopCPUProfile()
		_ = cpuFile.Close()
	}
	allocsFile, err := os.Create(filepath.Join(snapshotDir, fmt.Sprintf("allocs-%s.pb.gz", ts)))
	if err != nil {
		return err
	}
	defer allocsFile.Close()
	if err := pprof.Lookup("allocs").WriteTo(allocsFile, 0); err != nil {
		return err
	}
	goroutineFile, err := os.Create(filepath.Join(snapshotDir, fmt.Sprintf("goroutine-%s.txt", ts)))
	if err != nil {
		return err
	}
	defer goroutineFile.Close()
	if err := pprof.Lookup("goroutine").WriteTo(goroutineFile, 2); err != nil {
		return err
	}
	summary := map[string]any{
		"timestamp": time.Now().UnixMilli(),
		"kind":      "periodic",
		"hasCPU":    !sessionActive,
		"latest":    collectSample(),
	}
	return writeJSONFile(filepath.Join(snapshotDir, fmt.Sprintf("summary-%s.json", ts)), summary)
}

func normalizeConfig(cfg *Config) {
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./data/perf"
	}
	if cfg.LightSampleInterval <= 0 {
		cfg.LightSampleInterval = 15 * time.Second
	}
	if cfg.SnapshotInterval <= 0 {
		cfg.SnapshotInterval = 5 * time.Minute
	}
	if cfg.CPUProfileDuration <= 0 {
		cfg.CPUProfileDuration = 5 * time.Minute
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 72 * time.Hour
	}
}

func topFunctionsFromProfile(path, source string, limit int) ([]TopFunction, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	prof, err := pprofprofile.Parse(file)
	if err != nil {
		return nil, err
	}
	if len(prof.SampleType) == 0 || len(prof.Sample) == 0 {
		return []TopFunction{}, nil
	}

	sampleIndex := 0
	for i, item := range prof.SampleType {
		if strings.EqualFold(item.Type, "cpu") || strings.EqualFold(item.Type, "samples") {
			sampleIndex = i
			break
		}
	}

	type agg struct {
		name string
		flat int64
		cum  int64
	}
	byFunc := map[string]*agg{}
	var total int64
	for _, sample := range prof.Sample {
		if len(sample.Value) <= sampleIndex {
			continue
		}
		value := sample.Value[sampleIndex]
		if value <= 0 {
			continue
		}
		total += value
		seen := map[string]bool{}
		for locationIndex, location := range sample.Location {
			for _, line := range location.Line {
				fnName := resolveFunctionName(line.Function)
				if fnName == "" {
					continue
				}
				entry := byFunc[fnName]
				if entry == nil {
					entry = &agg{name: fnName}
					byFunc[fnName] = entry
				}
				if locationIndex == 0 {
					entry.flat += value
				}
				if !seen[fnName] {
					entry.cum += value
					seen[fnName] = true
				}
			}
		}
	}

	rows := make([]*agg, 0, len(byFunc))
	for _, item := range byFunc {
		rows = append(rows, item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].flat == rows[j].flat {
			if rows[i].cum == rows[j].cum {
				return rows[i].name < rows[j].name
			}
			return rows[i].cum > rows[j].cum
		}
		return rows[i].flat > rows[j].flat
	})

	if len(rows) > limit {
		rows = rows[:limit]
	}
	unit := prof.SampleType[sampleIndex].Unit
	result := make([]TopFunction, 0, len(rows))
	for _, item := range rows {
		result = append(result, TopFunction{
			Name:       item.name,
			Flat:       item.flat,
			Cumulative: item.cum,
			FlatPct:    formatPct(item.flat, total),
			CumPct:     formatPct(item.cum, total),
			Source:     source,
			Unit:       unit,
		})
	}
	return result, nil
}

func resolveFunctionName(fn *pprofprofile.Function) string {
	if fn == nil {
		return ""
	}
	if fn.Name != "" {
		return fn.Name
	}
	return fn.SystemName
}

func formatPct(value, total int64) string {
	if total <= 0 || value <= 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", float64(value)*100/float64(total))
}

func cloneSessionState(in *CPUSessionState) *CPUSessionState {
	if in == nil {
		return nil
	}
	copy := *in
	return &copy
}

func (m *Manager) MarshalJSON() ([]byte, error) {
	state := m.CurrentState()
	return json.Marshal(state)
}
