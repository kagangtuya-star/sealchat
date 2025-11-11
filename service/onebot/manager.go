package onebot

import (
	"sync"
	"time"

	"sealchat/utils"
)

type RuntimeStatus string

const (
	RuntimeStatusDisabled     RuntimeStatus = "disabled"
	RuntimeStatusDisconnected RuntimeStatus = "disconnected"
	RuntimeStatusConnecting   RuntimeStatus = "connecting"
	RuntimeStatusConnected    RuntimeStatus = "connected"
)

type RuntimeState struct {
	BotID     string        `json:"botId"`
	Status    RuntimeStatus `json:"status"`
	LastError string        `json:"lastError,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type Manager struct {
	cfg    *utils.OneBotConfig
	mu     sync.RWMutex
	states sync.Map
}

var (
	defaultManager *Manager
	managerOnce    sync.Once
)

func Init(cfg *utils.OneBotConfig) {
	managerOnce.Do(func() {
		defaultManager = &Manager{}
	})
	defaultManager.UpdateConfig(cfg)
	TriggerReverseRefresh()
}

func ManagerInstance() *Manager {
	if defaultManager == nil {
		defaultManager = &Manager{}
	}
	return defaultManager
}

func (m *Manager) UpdateConfig(cfg *utils.OneBotConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg == nil {
		m.cfg = &utils.OneBotConfig{}
		return
	}
	cloned := *cfg
	m.cfg = &cloned
}

func (m *Manager) Config() *utils.OneBotConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg == nil {
		return &utils.OneBotConfig{}
	}
	cloned := *m.cfg
	return &cloned
}

func (m *Manager) UpdateStatus(botID string, status RuntimeStatus, errMsg string) {
	if botID == "" {
		return
	}
	state := RuntimeState{
		BotID:  botID,
		Status: status,
	}
	if errMsg != "" {
		state.LastError = errMsg
	}
	state.UpdatedAt = time.Now()
	m.states.Store(botID, state)
}

func (m *Manager) GetStatus(botID string) RuntimeState {
	if botID == "" {
		return RuntimeState{Status: RuntimeStatusDisabled}
	}
	if v, ok := m.states.Load(botID); ok {
		if state, ok2 := v.(RuntimeState); ok2 {
			return state
		}
	}
	return RuntimeState{
		BotID:  botID,
		Status: RuntimeStatusDisconnected,
	}
}

func (m *Manager) SnapshotStates() map[string]RuntimeState {
	result := map[string]RuntimeState{}
	m.states.Range(func(key, value interface{}) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		state, ok := value.(RuntimeState)
		if !ok {
			return true
		}
		result[k] = state
		return true
	})
	return result
}

func (m *Manager) NotifyProfileChanged(botID string) {
	if botID == "" {
		return
	}
	// 预留钩子，未来可在此触发连接重建
}
