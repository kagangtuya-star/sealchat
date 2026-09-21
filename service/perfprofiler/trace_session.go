package perfprofiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	runtimeTrace "runtime/trace"
	"time"
)

var (
	ErrTraceSessionActive   = errors.New("runtime trace session already active")
	ErrTraceDurationInvalid = errors.New("runtime trace duration must be between 5 and 120 seconds")
)

const defaultTraceDuration = 30 * time.Second

func validateTraceDuration(duration time.Duration) (time.Duration, error) {
	if duration == 0 {
		return defaultTraceDuration, nil
	}
	if duration < 5*time.Second || duration > 120*time.Second {
		return 0, ErrTraceDurationInvalid
	}
	return duration, nil
}

func (m *Manager) StartTraceSession(duration time.Duration) (*TraceSessionState, error) {
	duration, err := validateTraceDuration(duration)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Enabled {
		return nil, ErrDisabled
	}
	if m.state.TraceSession != nil && m.state.TraceSession.Active {
		return nil, ErrTraceSessionActive
	}
	if err := ensureDir(filepath.Join(m.cfg.OutputDir, "sessions")); err != nil {
		return nil, err
	}
	now := time.Now()
	sessionID := now.Format("20060102-150405.000")
	fileName := fmt.Sprintf("trace-session-%s.out", sessionID)
	filePath := filepath.Join(m.cfg.OutputDir, "sessions", fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	if err := runtimeTrace.Start(file); err != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &TraceSessionState{
		SessionID: sessionID,
		Active:    true,
		StartedAt: now.UnixMilli(),
		EndsAt:    now.Add(duration).UnixMilli(),
		FileName:  fileName,
	}
	m.state.TraceSession = state
	m.traceCancel = cancel
	m.wg.Add(1)
	go m.finishTraceSession(ctx, file, filePath, sessionID, duration)
	return cloneTraceSessionState(state), nil
}

func (m *Manager) StopTraceSession() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	cancel := m.traceCancel
	m.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (m *Manager) finishTraceSession(ctx context.Context, file *os.File, filePath, sessionID string, duration time.Duration) {
	defer m.wg.Done()
	autoStopped := true
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		autoStopped = false
	case <-timer.C:
	}
	runtimeTrace.Stop()
	closeErr := file.Close()
	info, statErr := os.Stat(filePath)
	var size int64
	if info != nil {
		size = info.Size()
	}
	lastError := ""
	if closeErr != nil {
		lastError = closeErr.Error()
	} else if statErr != nil {
		lastError = statErr.Error()
	}
	meta := map[string]any{
		"sessionId":   sessionID,
		"fileName":    filepath.Base(filePath),
		"fileSize":    size,
		"autoStopped": autoStopped,
		"finishedAt":  time.Now().UnixMilli(),
	}
	if lastError != "" {
		meta["lastError"] = lastError
	}
	_ = writeJSONFile(filepath.Join(m.cfg.OutputDir, "sessions", fmt.Sprintf("trace-session-%s.json", sessionID)), meta)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.TraceSession != nil && m.state.TraceSession.SessionID == sessionID {
		m.state.TraceSession.Active = false
		m.state.TraceSession.FileSize = size
		m.state.TraceSession.AutoStopped = autoStopped
		m.state.TraceSession.LastError = lastError
	}
	m.traceCancel = nil
}

func cloneTraceSessionState(in *TraceSessionState) *TraceSessionState {
	if in == nil {
		return nil
	}
	copy := *in
	return &copy
}
