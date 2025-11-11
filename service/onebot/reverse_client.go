package onebot

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"

	"sealchat/model"
	"sealchat/protocol/onebotv11"
	"sealchat/utils"
)

var (
	refreshOnce sync.Once
	refreshCh   chan struct{}
)

func TriggerReverseRefresh() {
	refreshOnce.Do(func() {
		refreshCh = make(chan struct{}, 1)
		go reverseRefreshWorker()
	})
	select {
	case refreshCh <- struct{}{}:
	default:
	}
}

func reverseRefreshWorker() {
	for range refreshCh {
		if err := refreshReverseClients(); err != nil {
			log.Printf("onebot reverse refresh failed: %v", err)
		}
	}
}

type reverseRunner struct {
	profile *model.BotProfileModel
	stop    chan struct{}
	wg      sync.WaitGroup
}

var (
	runners   sync.Map
	dialer    = websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	connKinds = []ConnKind{ConnKindAPI, ConnKindEvent, ConnKindUniversal}
)

func refreshReverseClients() error {
	cfg := ManagerInstance().Config()
	profiles, err := model.BotProfileList()
	if err != nil {
		return err
	}

	desired := map[string]*model.BotProfileModel{}
	for _, profile := range profiles {
		if profile == nil || !profile.Enabled {
			continue
		}
		if profile.ConnMode != model.BotConnectionModeReverseWS {
			continue
		}
		desired[profile.ID] = profile
	}

	runners.Range(func(key, value interface{}) bool {
		id, _ := key.(string)
		if _, ok := desired[id]; ok {
			return true
		}
		if runner, ok := value.(*reverseRunner); ok {
			runner.stopRunner()
		}
		runners.Delete(key)
		return true
	})

	for id, profile := range desired {
		if _, exists := runners.Load(id); exists {
			continue
		}
		runner := &reverseRunner{
			profile: profile,
			stop:    make(chan struct{}),
		}
		runners.Store(id, runner)
		runner.start(cfg)
	}

	return nil
}

func (r *reverseRunner) start(cfg *utils.OneBotConfig) {
	for _, kind := range connKinds {
		endpoints := r.collectEndpoints(cfg, kind)
		if len(endpoints) == 0 {
			continue
		}
		for _, endpoint := range endpoints {
			ep := endpoint
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				r.loop(ep, kind, cfg)
			}()
		}
	}
}

func (r *reverseRunner) stopRunner() {
	close(r.stop)
	r.wg.Wait()
}

func (r *reverseRunner) collectEndpoints(cfg *utils.OneBotConfig, kind ConnKind) []string {
	switch kind {
	case ConnKindAPI:
		if len(r.profile.ReverseAPIEndpoints) > 0 {
			return r.profile.ReverseAPIEndpoints
		}
		return cfg.WSReverse.APIEndpoints
	case ConnKindEvent:
		if len(r.profile.ReverseEventURLs) > 0 {
			return r.profile.ReverseEventURLs
		}
		return cfg.WSReverse.EventEndpoints
	case ConnKindUniversal:
		if len(r.profile.ReverseUniversalURLs) > 0 {
			return r.profile.ReverseUniversalURLs
		}
		return cfg.WSReverse.UniversalEndpoints
	default:
		return nil
	}
}

func (r *reverseRunner) loop(endpoint string, kind ConnKind, cfg *utils.OneBotConfig) {
	manager := ManagerInstance()
	backoff := time.Duration(r.profile.ReverseReconnectSec)
	if backoff <= 0 {
		backoff = time.Duration(cfg.WSReverse.ReconnectIntervalSeconds)
	}
	if backoff <= 0 {
		backoff = 10
	}
	delay := time.Second * backoff

	for {
		select {
		case <-r.stop:
			return
		default:
		}

		manager.UpdateStatus(r.profile.ID, RuntimeStatusConnecting, "")
		if err := r.connectOnce(endpoint, kind, cfg); err != nil {
			manager.UpdateStatus(r.profile.ID, RuntimeStatusDisconnected, err.Error())
			log.Printf("onebot reverse connect failed (%s %s): %v", r.profile.Name, endpoint, err)
		} else {
			manager.UpdateStatus(r.profile.ID, RuntimeStatusDisconnected, "")
		}

		select {
		case <-r.stop:
			return
		case <-time.After(delay):
		}
	}
}

func (r *reverseRunner) connectOnce(endpoint string, kind ConnKind, cfg *utils.OneBotConfig) error {
	header := http.Header{}
	token := strings.TrimSpace(r.profile.AccessToken)
	if token == "" {
		token = strings.TrimSpace(cfg.Auth.AccessToken)
	}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	header.Set("X-Client-Role", string(kind))
	header.Set("X-Self-ID", r.profile.UserID)

	conn, _, err := dialer.Dial(endpoint, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	ManagerInstance().UpdateStatus(r.profile.ID, RuntimeStatusConnected, "")

	switch kind {
	case ConnKindEvent, ConnKindUniversal:
		return r.consumeEvents(conn)
	default:
		return r.keepAlive(conn)
	}
}

func (r *reverseRunner) consumeEvents(conn *websocket.Conn) error {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var event onebotv11.Event
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = getDispatcher().HandleEvent(ctx, r.profile, &event)
		cancel()
	}
}

func (r *reverseRunner) keepAlive(conn *websocket.Conn) error {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return err
		}
	}
}
