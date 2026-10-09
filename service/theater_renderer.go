package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sync"
	"time"

	"sealchat/utils"
)

const TheaterRendererVersion = 1
const theaterRendererTTL = 60 * time.Second
const theaterTaskTTL = 2 * time.Minute

type TheaterRendererRegistration struct {
	TheaterScope
	RendererID    string              `json:"rendererId"`
	UserID        string              `json:"userId"`
	ActiveSceneID string              `json:"activeSceneId"`
	Revision      int64               `json:"revision"`
	Viewport      TheaterRendererView `json:"viewport"`
	Capabilities  []string            `json:"capabilities"`
	Catalog       map[string]any      `json:"catalog,omitempty"`
	Version       int                 `json:"version"`
}
type TheaterRendererView struct {
	Width             int                   `json:"width"`
	Height            int                   `json:"height"`
	Camera            TheaterRendererCamera `json:"camera"`
	SelectedObjectIDs []string              `json:"selectedObjectIds,omitempty"`
}
type TheaterRendererCamera struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}
type TheaterRendererCommand struct {
	Version          int          `json:"version"`
	RequestID        string       `json:"requestId"`
	RendererID       string       `json:"rendererId"`
	Scope            TheaterScope `json:"scope"`
	SceneID          string       `json:"sceneId"`
	ExpectedRevision int64        `json:"expectedRevision"`
	ExpiresAt        int64        `json:"expiresAt"`
	Operation        string       `json:"operation"`
	Payload          any          `json:"payload"`
}
type TheaterRendererReply struct {
	Version    int             `json:"version"`
	RequestID  string          `json:"requestId"`
	RendererID string          `json:"rendererId"`
	Scope      TheaterScope    `json:"scope"`
	SceneID    string          `json:"sceneId"`
	Revision   int64           `json:"revision"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}
type TheaterCaptureResult struct {
	CaptureID          string         `json:"captureId"`
	RendererID         string         `json:"rendererId"`
	SceneID            string         `json:"sceneId"`
	Revision           int64          `json:"revision"`
	Status             string         `json:"status"`
	Width              int            `json:"width"`
	Height             int            `json:"height"`
	MimeType           string         `json:"mimeType"`
	CoordinateMapping  map[string]any `json:"coordinateMapping"`
	IncludedLayers     []string       `json:"includedLayers"`
	MissingLayers      []string       `json:"missingLayers"`
	UnsupportedObjects []string       `json:"unsupportedObjects"`
	Warnings           []string       `json:"warnings"`
	Data               string         `json:"data,omitempty"`
}
type TheaterRendererTask struct {
	RequestID                               string                `json:"requestId"`
	RendererID                              string                `json:"rendererId"`
	Scope                                   TheaterScope          `json:"scope"`
	SceneID                                 string                `json:"sceneId"`
	Revision                                int64                 `json:"revision"`
	Status                                  string                `json:"status"`
	Error                                   string                `json:"error,omitempty"`
	Result                                  json.RawMessage       `json:"result,omitempty"`
	Capture                                 *TheaterCaptureResult `json:"capture,omitempty"`
	CompletedStepIDs                        []string              `json:"completedStepIds,omitempty"`
	InFlightStepID                          string                `json:"inFlightStepId,omitempty"`
	PartialSideEffectsPossible              bool                  `json:"partialSideEffectsPossible,omitempty"`
	actorID, keyID, connectionID, operation string
	createdAt, deadline                     time.Time
	done                                    chan struct{}
	timer                                   *time.Timer
	steps                                   map[string]bool
	stepBusy, stepAttempted                 bool
	terminalStatus, terminalError           string
	stepCancel                              context.CancelFunc
	step                                    func(context.Context, string, string, int64) (any, error)
	maxEdge, maxBytes                       int
}
type theaterRendererEntry struct {
	registration TheaterRendererRegistration
	connectionID string
	heartbeat    time.Time
	send         func(TheaterRendererCommand) error
}

func (t *TheaterRendererTask) Operation() string { return t.operation }

type TheaterRendererBroker struct {
	mu        sync.Mutex
	renderers map[string]*theaterRendererEntry
	tasks     map[string]*TheaterRendererTask
}

func NewTheaterRendererBroker() *TheaterRendererBroker {
	return &TheaterRendererBroker{renderers: map[string]*theaterRendererEntry{}, tasks: map[string]*TheaterRendererTask{}}
}

var TheaterRenderers = NewTheaterRendererBroker()

func theaterSameRoom(a, b TheaterScope) bool {
	return a.WorldID == b.WorldID && a.ScopeType == b.ScopeType && a.ChannelID == b.ChannelID
}
func theaterSameRendererContext(a, b TheaterScope) bool {
	return theaterSameRoom(a, b) && a.InputChannelID == b.InputChannelID
}
func rendererError(code, message string) error { return newTheaterError(code, message, 400, nil) }
func (b *TheaterRendererBroker) pruneLocked() {
	now := time.Now()
	for id, r := range b.renderers {
		if now.Sub(r.heartbeat) > theaterRendererTTL {
			delete(b.renderers, id)
			b.failConnectionLocked(r.connectionID, "renderer_unavailable")
		}
	}
	for id, t := range b.tasks {
		if now.Sub(t.createdAt) > theaterTaskTTL && theaterTaskTerminal(t.Status) {
			delete(b.tasks, id)
		}
	}
}
func theaterTaskTerminal(s string) bool { return s == "completed" || s == "failed" || s == "cancelled" }
func (b *TheaterRendererBroker) finishLocked(t *TheaterRendererTask, status, err string) {
	if theaterTaskTerminal(t.Status) || (t.Status == "stopping" && t.stepBusy) {
		return
	}
	if t.stepBusy {
		// An already-running server step may have committed before cancellation.
		// Do not publish a terminal result until that step has resolved.
		t.Status, t.Error = "stopping", err
		t.terminalStatus, t.terminalError = status, err
		t.PartialSideEffectsPossible = true
		if t.stepCancel != nil {
			t.stepCancel() // Best effort: committed side effects still require reconciliation.
		}
		t.step = nil
		if t.timer != nil {
			t.timer.Stop()
		}
		return
	}
	t.Status, t.Error = status, err
	if status != "completed" && t.stepAttempted {
		t.PartialSideEffectsPossible = true
	}
	t.step = nil
	if t.timer != nil {
		t.timer.Stop()
	}
	close(t.done)
}
func (b *TheaterRendererBroker) failConnectionLocked(connectionID, reason string) {
	for _, t := range b.tasks {
		if t.connectionID == connectionID {
			b.finishLocked(t, "failed", reason)
		}
	}
}
func (b *TheaterRendererBroker) Disconnect(connectionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, r := range b.renderers {
		if r.connectionID == connectionID {
			delete(b.renderers, id)
		}
	}
	b.failConnectionLocked(connectionID, "renderer_unavailable")
}
func (b *TheaterRendererBroker) Register(actorID, connectionID string, r TheaterRendererRegistration, send func(TheaterRendererCommand) error) error {
	scope, err := NormalizeTheaterMCPScope(actorID, r.TheaterScope)
	if err != nil {
		return err
	}
	if r.Version != TheaterRendererVersion || r.UserID != actorID || r.RendererID == "" || len(r.RendererID) > 128 || r.Revision < 0 || r.Viewport.Width < 1 || r.Viewport.Width > 16384 || r.Viewport.Height < 1 || r.Viewport.Height > 16384 || !validRendererCamera(r.Viewport.Camera) {
		return theaterPayloadError("renderer 登记无效")
	}
	if len(r.Capabilities) > 3 || len(r.Viewport.SelectedObjectIDs) > 200 {
		return theaterPayloadError("renderer capabilities/selection 超限")
	}
	for _, c := range r.Capabilities {
		if c != "view" && c != "capture" && c != "execute" {
			return theaterPayloadError("renderer capability 无效")
		}
	}
	raw, _ := json.Marshal(r.Catalog)
	if len(raw) > 128<<10 {
		return theaterPayloadError("renderer catalog 超限")
	}
	r.TheaterScope = scope
	snapshot, err := GetTheaterSnapshot(context.Background(), actorID, scope.WorldID, scope.ChannelID, TheaterSnapshotOptions{})
	if err != nil {
		return err
	}
	if r.Revision > snapshot.Revision || snapshot.Snapshot.ActiveSceneID == nil || r.ActiveSceneID != *snapshot.Snapshot.ActiveSceneID {
		return rendererError("renderer_scene_mismatch", "renderer 必须登记当前场景与已同步版本")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	if old := b.renderers[r.RendererID]; old != nil && (old.registration.UserID != actorID || old.connectionID != connectionID) {
		return rendererError("renderer_denied", "rendererId 已绑定其他连接")
	}
	if len(b.renderers) >= 128 && b.renderers[r.RendererID] == nil {
		return rendererError("renderer_limit", "renderer 数量超限")
	}
	// One scope per renderer: a context change invalidates all outstanding tasks.
	if old := b.renderers[r.RendererID]; old != nil && !theaterSameRendererContext(old.registration.TheaterScope, scope) {
		b.failConnectionLocked(connectionID, "scope_changed")
	}
	b.renderers[r.RendererID] = &theaterRendererEntry{r, connectionID, time.Now(), send}
	return nil
}
func validRendererCamera(c TheaterRendererCamera) bool {
	return !math.IsNaN(c.X) && !math.IsInf(c.X, 0) && !math.IsNaN(c.Y) && !math.IsInf(c.Y, 0) && !math.IsNaN(c.Zoom) && !math.IsInf(c.Zoom, 0) && math.Abs(c.X) <= 1_000_000 && math.Abs(c.Y) <= 1_000_000 && c.Zoom >= 0.01 && c.Zoom <= 100
}
func (b *TheaterRendererBroker) List(actorID string, s TheaterScope) []TheaterRendererRegistration {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	out := []TheaterRendererRegistration{}
	for _, r := range b.renderers {
		if r.registration.UserID == actorID && theaterSameRoom(r.registration.TheaterScope, s) {
			v := r.registration
			v.Catalog = nil
			out = append(out, v)
		}
	}
	return out
}
func (b *TheaterRendererBroker) Catalog(actorID string, s TheaterScope) []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	out := []map[string]any{}
	for _, r := range b.renderers {
		if r.registration.UserID == actorID && theaterSameRoom(r.registration.TheaterScope, s) {
			out = append(out, map[string]any{"rendererId": r.registration.RendererID, "catalog": r.registration.Catalog})
		}
	}
	return out
}
func (b *TheaterRendererBroker) Start(actorID, keyID string, s TheaterScope, rendererID, sceneID string, revision int64, operation string, payload any, stepIDs []string, step func(context.Context, string, string, int64) (any, error)) (*TheaterRendererTask, error) {
	b.mu.Lock()
	b.pruneLocked()
	r := b.renderers[rendererID]
	if r == nil || r.registration.UserID != actorID || !theaterSameRendererContext(r.registration.TheaterScope, s) {
		b.mu.Unlock()
		return nil, rendererError("renderer_unavailable", "没有同账号、同作用域的授权 renderer")
	}
	cap := "view"
	timeout := 8 * time.Second
	if operation == "capture" {
		cap = "capture"
		timeout = 15 * time.Second
	} else if operation == "execute" {
		cap = "execute"
		timeout = 60 * time.Second
	}
	if !theaterContains(r.registration.Capabilities, cap) {
		b.mu.Unlock()
		return nil, rendererError("renderer_unsupported", "renderer 未授权此能力")
	}
	if r.registration.ActiveSceneID != sceneID {
		b.mu.Unlock()
		return nil, rendererError("renderer_scene_mismatch", "renderer 目标场景不匹配")
	}
	activeUser, activeGlobal := 0, 0
	for _, t := range b.tasks {
		if !theaterTaskTerminal(t.Status) {
			activeGlobal++
			if t.actorID == actorID {
				activeUser++
			}
			if t.RendererID == rendererID {
				b.mu.Unlock()
				return nil, rendererError("renderer_busy", "renderer 有未完成任务")
			}
		}
	}
	if activeUser >= 2 || activeGlobal >= 8 || len(b.tasks) >= 128 {
		b.mu.Unlock()
		return nil, rendererError("renderer_limit", "远程任务并发或保留数量超限")
	}
	id := utils.NewID()
	now := time.Now()
	t := &TheaterRendererTask{RequestID: id, RendererID: rendererID, Scope: s, SceneID: sceneID, Revision: revision, Status: "pending", actorID: actorID, keyID: keyID, connectionID: r.connectionID, operation: operation, createdAt: now, deadline: now.Add(timeout), done: make(chan struct{}), step: step, steps: map[string]bool{}}
	t.maxEdge, t.maxBytes = 1600, 2<<20
	if operation == "capture" {
		if values, ok := payload.(map[string]any); ok {
			if edge, ok := values["maxEdge"].(int); ok {
				t.maxEdge = edge
			}
			if size, ok := values["maxBytes"].(int); ok {
				t.maxBytes = size
			}
		}
	}
	b.tasks[id] = t
	for _, stepID := range stepIDs {
		t.steps[stepID] = false
	}
	command := TheaterRendererCommand{TheaterRendererVersion, id, rendererID, s, sceneID, revision, t.deadline.UnixMilli(), operation, payload}
	t.timer = time.AfterFunc(timeout, func() {
		b.mu.Lock()
		timedOut := !theaterTaskTerminal(t.Status)
		if timedOut {
			b.finishLocked(t, "failed", "renderer_timeout")
		}
		b.mu.Unlock()
		if !timedOut {
			return
		}
		cancel := command
		cancel.Operation = "cancel"
		cancel.Payload = nil
		_ = r.send(cancel)
	})
	b.mu.Unlock()
	if err := r.send(command); err != nil {
		b.mu.Lock()
		b.finishLocked(t, "failed", "renderer_unavailable")
		b.mu.Unlock()
	}
	return b.Status(actorID, keyID, s, id)
}
func cloneRendererTask(t *TheaterRendererTask) *TheaterRendererTask {
	c := *t
	c.Result = append(json.RawMessage{}, t.Result...)
	c.CompletedStepIDs = append([]string(nil), t.CompletedStepIDs...)
	if t.Capture != nil {
		v := *t.Capture
		c.Capture = &v
	}
	return &c
}
func (b *TheaterRendererBroker) Status(actorID, keyID string, s TheaterScope, id string) (*TheaterRendererTask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	t := b.tasks[id]
	if t == nil || t.actorID != actorID || t.keyID != keyID || !theaterSameRendererContext(t.Scope, s) {
		return nil, rendererError("task_not_found", "任务不存在或无权访问")
	}
	return cloneRendererTask(t), nil
}
func (b *TheaterRendererBroker) Wait(ctx context.Context, actorID, keyID string, s TheaterScope, id string) (*TheaterRendererTask, error) {
	t, err := b.Status(actorID, keyID, s, id)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return b.Status(actorID, keyID, s, id)
	}
}
func (b *TheaterRendererBroker) Cancel(actorID, keyID string, s TheaterScope, id string) (*TheaterRendererTask, error) {
	b.mu.Lock()
	b.pruneLocked()
	t := b.tasks[id]
	if t == nil || t.actorID != actorID || t.keyID != keyID || !theaterSameRendererContext(t.Scope, s) {
		b.mu.Unlock()
		return nil, rendererError("task_not_found", "任务不存在或无权访问")
	}
	r := b.renderers[t.RendererID]
	wasTerminal := theaterTaskTerminal(t.Status) || t.Status == "stopping"
	command := TheaterRendererCommand{Version: TheaterRendererVersion, RequestID: id, RendererID: t.RendererID, Scope: s, SceneID: t.SceneID, ExpectedRevision: t.Revision, Operation: "cancel", ExpiresAt: time.Now().Add(time.Second).UnixMilli()}
	b.finishLocked(t, "cancelled", "")
	b.mu.Unlock()
	if r != nil && !wasTerminal {
		_ = r.send(command)
	}
	return b.Status(actorID, keyID, s, id)
}
func (b *TheaterRendererBroker) taskForReplyLocked(actorID, connectionID string, r TheaterRendererReply) (*TheaterRendererTask, error) {
	b.pruneLocked()
	t := b.tasks[r.RequestID]
	if r.Version != TheaterRendererVersion || t == nil || t.actorID != actorID || t.connectionID != connectionID || t.RendererID != r.RendererID || !theaterSameRendererContext(t.Scope, r.Scope) || theaterTaskTerminal(t.Status) || t.Status == "stopping" || time.Now().After(t.deadline) {
		return nil, rendererError("stale_command", "命令已过期、取消或作用域不匹配")
	}
	return t, nil
}
func (b *TheaterRendererBroker) Reply(actorID, connectionID string, r TheaterRendererReply) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, err := b.taskForReplyLocked(actorID, connectionID, r)
	if err != nil {
		return err
	}
	if r.Status == "running" {
		if r.SceneID != t.SceneID || r.Revision != t.Revision {
			return rendererError("renderer_revision_mismatch", "renderer 状态版本不匹配")
		}
		t.Status = "running"
		return nil
	}
	if r.Status == "failed" {
		b.finishLocked(t, "failed", r.Error)
		return nil
	}
	if r.Status == "cancelled" {
		b.finishLocked(t, "cancelled", "")
		return nil
	}
	if r.Status != "completed" {
		return theaterPayloadError("renderer response status 无效")
	}
	if t.operation == "execute" {
		for _, done := range t.steps {
			if !done {
				return theaterPayloadError("执行步骤尚未完成")
			}
		}
		if t.stepBusy || r.Revision != t.Revision || r.SceneID != t.SceneID {
			return theaterPayloadError("执行仍在运行或 revision 不匹配")
		}
	}
	if t.operation != "execute" && (r.SceneID != t.SceneID || r.Revision != t.Revision) {
		b.finishLocked(t, "failed", "renderer_revision_mismatch")
		return nil
	}
	if t.operation == "capture" {
		var capture TheaterCaptureResult
		if decodeStrictJSON(r.Result, &capture) != nil {
			return theaterPayloadError("capture result 无效")
		}
		if err := validateTheaterCapture(&capture, t); err != nil {
			b.finishLocked(t, "failed", "capture_invalid")
			return err
		}
		t.Capture = &capture
	} else {
		if len(r.Result) > 256<<10 {
			return theaterPayloadError("renderer result 超限")
		}
		t.Result = append(json.RawMessage{}, r.Result...)
	}
	t.Revision = r.Revision
	b.finishLocked(t, "completed", "")
	return nil
}
func validateTheaterCapture(c *TheaterCaptureResult, t *TheaterRendererTask) error {
	if t.maxEdge == 0 {
		t.maxEdge = 1600
	}
	if t.maxBytes == 0 {
		t.maxBytes = 2 << 20
	}
	if c.CaptureID != t.RequestID || c.RendererID != t.RendererID || c.SceneID != t.SceneID || c.Revision != t.Revision || (c.Status != "complete" && c.Status != "partial") || c.Width < 1 || c.Height < 1 || c.Width > 1600 || c.Height > 1600 {
		return theaterPayloadError("capture metadata 无效")
	}
	if c.Width > t.maxEdge || c.Height > t.maxEdge || len(c.IncludedLayers) > 256 || len(c.MissingLayers) > 5000 || len(c.UnsupportedObjects) > 5000 || len(c.Warnings) > 256 || c.CoordinateMapping == nil {
		return theaterPayloadError("capture metadata 超限或缺失坐标映射")
	}
	var mapping struct {
		WorldUnitPx   int    `json:"worldUnitPx"`
		Anchor        string `json:"anchor"`
		WorldOriginPx struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"worldOriginPx"`
		PixelsPerWorldUnit float64               `json:"pixelsPerWorldUnit"`
		Camera             TheaterRendererCamera `json:"camera"`
		Crop               struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"crop"`
		EffectDesignSize struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"effectDesignSize"`
	}
	raw, err := json.Marshal(c.CoordinateMapping)
	if err != nil || decodeStrictJSON(raw, &mapping) != nil || mapping.WorldUnitPx != 24 || mapping.Anchor != "center" || !validRendererCamera(mapping.Camera) || mapping.PixelsPerWorldUnit <= 0 || mapping.Crop.Width <= 0 || mapping.Crop.Height <= 0 || mapping.Crop.X < 0 || mapping.Crop.Y < 0 || mapping.EffectDesignSize.Width != 1920 || mapping.EffectDesignSize.Height != 1080 {
		return theaterPayloadError("capture coordinateMapping 无效")
	}
	for _, value := range []float64{mapping.WorldOriginPx.X, mapping.WorldOriginPx.Y, mapping.PixelsPerWorldUnit, mapping.Crop.X, mapping.Crop.Y, mapping.Crop.Width, mapping.Crop.Height} {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 1e9 {
			return theaterPayloadError("capture coordinateMapping 必须为有限范围数")
		}
	}
	if c.Status == "complete" && (len(c.MissingLayers) > 0 || len(c.UnsupportedObjects) > 0) {
		return theaterPayloadError("缺失图层不得标记 complete")
	}
	if c.MimeType != "image/png" && c.MimeType != "image/jpeg" {
		return theaterPayloadError("capture mimeType 无效")
	}
	if len(c.Data) > base64.StdEncoding.EncodedLen(t.maxBytes) {
		return theaterPayloadError("capture 超过 2 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return theaterPayloadError("capture base64 无效")
	}
	if len(data) > t.maxBytes {
		return theaterPayloadError("capture 超过请求大小限制")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != c.Width || cfg.Height != c.Height || "image/"+format != c.MimeType {
		return theaterPayloadError("capture 图片尺寸/格式无效")
	}
	return nil
}

// Each planned leaf may run once. Payload is never accepted from the browser.
func (b *TheaterRendererBroker) Step(ctx context.Context, actorID, connectionID string, r TheaterRendererReply, stepID string) (any, error) {
	b.mu.Lock()
	t, err := b.taskForReplyLocked(actorID, connectionID, r)
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}
	done, exists := t.steps[stepID]
	if t.operation != "execute" || t.step == nil || !exists || done || t.stepBusy || r.Revision != t.Revision || r.SceneID != t.SceneID {
		b.mu.Unlock()
		return nil, rendererError("step_rejected", "步骤已执行、不存在、并发或 revision 不匹配")
	}
	t.stepBusy, t.stepAttempted = true, true
	t.InFlightStepID = stepID
	t.Status = "running"
	stepCtx, cancel := context.WithCancel(ctx)
	t.stepCancel = cancel
	executor := t.step
	b.mu.Unlock()
	result, err := executor(stepCtx, t.RequestID, stepID, r.Revision)
	cancel()
	b.mu.Lock()
	t.stepCancel = nil
	t.stepBusy = false
	t.InFlightStepID = ""
	if err == nil {
		// Keep confirmed server outcomes even if the renderer was cancelled or lost
		// while the step was executing. A failed visual follow-up must not replay it.
		t.steps[stepID] = true
		t.CompletedStepIDs = append(t.CompletedStepIDs, stepID)
		if action, ok := result.(*TheaterActionResult); ok && action.Mutation != nil {
			t.Revision = action.Mutation.Revision
			if action.Mutation.Type == TheaterMutationSceneApply {
				var p theaterSceneApplyPayload
				if json.Unmarshal(action.Mutation.Payload, &p) == nil {
					t.SceneID = p.SceneID
				}
			}
		}
	}
	if t.terminalStatus != "" {
		b.finishLocked(t, t.terminalStatus, t.terminalError)
	} else if err != nil {
		b.finishLocked(t, "failed", "action_failed")
	}
	b.mu.Unlock()
	return result, err
}
