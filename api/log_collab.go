package api

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

// LOG 设计协作：每个打开工作台的连接在工程内登记一条 presence（章节、焦点消息、正在编辑的消息），
// 心跳刷新、超时清理。正在编辑的消息即编辑锁：同一消息同时只授予一位用户。
// 状态只存在于当前进程内存，多实例部署时各实例互不可见；消息本身的版本 CAS 仍是最终保护。
const (
	logPresenceTTL           = 45 * time.Second
	logPresenceAccessRecheck = time.Minute
	logPresenceSweep         = 10 * time.Second
	logPresenceMaxPerProject = 64
	logPresenceMaxIDLength   = 100
)

type logPresenceView struct {
	SessionID        string `json:"sessionId"`
	UserID           string `json:"userId"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar,omitempty"`
	SectionID        string `json:"sectionId,omitempty"`
	MessageID        string `json:"messageId,omitempty"`
	EditingMessageID string `json:"editingMessageId,omitempty"`
	// Activity: viewing（只在浏览）、selecting（选中了消息）、editing（持有编辑锁）。
	Activity  string `json:"activity"`
	UpdatedAt int64  `json:"updatedAt"`
}

type logPresenceLock struct {
	MessageID    string `json:"messageId"`
	Granted      bool   `json:"granted"`
	HolderUserID string `json:"holderUserId,omitempty"`
	HolderName   string `json:"holderName,omitempty"`
}

type logPresenceUpdateResult struct {
	SessionID string            `json:"sessionId"`
	Presences []logPresenceView `json:"presences"`
	Lock      *logPresenceLock  `json:"lock,omitempty"`
}

type logPresenceRequest struct {
	ProjectID        string `json:"projectId"`
	SectionID        string `json:"sectionId"`
	MessageID        string `json:"messageId"`
	EditingMessageID string `json:"editingMessageId"`
}

type logPresenceEntry struct {
	view    logPresenceView
	seen    time.Time
	checked time.Time
}

type logPresenceIdentity struct {
	UserID string
	Name   string
	Avatar string
}

type logCollabRegistry struct {
	mu       sync.Mutex
	now      func() time.Time
	newID    func() string
	projects map[string]map[*WsSyncConn]*logPresenceEntry
	sessions map[*WsSyncConn]string
	sweeper  sync.Once
}

var errLogPresenceFull = &service.LogError{Code: "RATE_LIMITED", Message: "同时打开该 LOG 的连接过多"}
var errLogPresenceStale = &service.LogError{Code: "PERMISSION_DENIED", Message: "协作状态已失效，请重新进入"}

func newLogCollabRegistry() *logCollabRegistry {
	return &logCollabRegistry{
		now:      time.Now,
		newID:    func() string { return "lp-" + utils.NewID() },
		projects: map[string]map[*WsSyncConn]*logPresenceEntry{},
		sessions: map[*WsSyncConn]string{},
	}
}

var logCollab = newLogCollabRegistry()

func logPresenceActivity(v *logPresenceView) string {
	if v.EditingMessageID != "" {
		return "editing"
	}
	if v.MessageID != "" {
		return "selecting"
	}
	return "viewing"
}

// needsAccessCheck 为 true 时调用方须在不持锁的情况下重新校验设计权限。
func (r *logCollabRegistry) needsAccessCheck(conn *WsSyncConn, projectID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.projects[projectID][conn]
	return entry == nil || r.now().Sub(entry.checked) >= logPresenceAccessRecheck
}

func (r *logCollabRegistry) pruneLocked(now time.Time) []string {
	var changed []string
	for projectID, entries := range r.projects {
		removed := false
		for conn, entry := range entries {
			if now.Sub(entry.seen) >= logPresenceTTL {
				delete(entries, conn)
				removed = true
			}
		}
		if len(entries) == 0 {
			delete(r.projects, projectID)
		}
		if removed {
			changed = append(changed, projectID)
		}
	}
	r.forgetIdleSessionsLocked()
	return changed
}

func (r *logCollabRegistry) forgetIdleSessionsLocked() {
	for conn := range r.sessions {
		active := false
		for _, entries := range r.projects {
			if entries[conn] != nil {
				active = true
				break
			}
		}
		if !active {
			delete(r.sessions, conn)
		}
	}
}

func (r *logCollabRegistry) snapshotLocked(projectID string) []logPresenceView {
	entries := r.projects[projectID]
	out := make([]logPresenceView, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

// apply 记录一次 presence 更新并仲裁编辑锁。返回值 changed 表示需要向其他连接广播的工程。
func (r *logCollabRegistry) apply(conn *WsSyncConn, who logPresenceIdentity, input logPresenceRequest, accessChecked bool) (*logPresenceUpdateResult, []string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	changed := r.pruneLocked(now)
	entries := r.projects[input.ProjectID]
	entry := entries[conn]
	if entry == nil {
		if !accessChecked {
			return nil, changed, errLogPresenceStale
		}
		if len(entries) >= logPresenceMaxPerProject {
			return nil, changed, errLogPresenceFull
		}
		if entries == nil {
			entries = map[*WsSyncConn]*logPresenceEntry{}
			r.projects[input.ProjectID] = entries
		}
		session := r.sessions[conn]
		if session == "" {
			session = r.newID()
			r.sessions[conn] = session
		}
		entry = &logPresenceEntry{view: logPresenceView{SessionID: session}}
		entries[conn] = entry
	}
	if accessChecked {
		entry.checked = now
	}
	entry.seen = now
	before := entry.view
	next := logPresenceView{
		SessionID: entry.view.SessionID,
		UserID:    who.UserID,
		Name:      who.Name,
		Avatar:    who.Avatar,
		SectionID: input.SectionID,
		MessageID: input.MessageID,
		UpdatedAt: entry.view.UpdatedAt,
	}
	var lock *logPresenceLock
	if input.EditingMessageID != "" {
		lock = &logPresenceLock{MessageID: input.EditingMessageID, Granted: true}
		for other, e := range entries {
			if other == conn || e.view.UserID == who.UserID || e.view.EditingMessageID != input.EditingMessageID {
				continue
			}
			lock.Granted = false
			lock.HolderUserID = e.view.UserID
			lock.HolderName = e.view.Name
			break
		}
		if lock.Granted {
			next.EditingMessageID = input.EditingMessageID
		}
	}
	next.Activity = logPresenceActivity(&next)
	compare := before
	compare.UpdatedAt = next.UpdatedAt
	if compare != next {
		next.UpdatedAt = now.UnixMilli()
		changed = appendUniqueString(changed, input.ProjectID)
	}
	entry.view = next
	return &logPresenceUpdateResult{SessionID: next.SessionID, Presences: r.snapshotLocked(input.ProjectID), Lock: lock}, changed, nil
}

func appendUniqueString(list []string, value string) []string {
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}

func (r *logCollabRegistry) leave(conn *WsSyncConn, projectID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := r.projects[projectID]
	if entries[conn] == nil {
		return false
	}
	delete(entries, conn)
	if len(entries) == 0 {
		delete(r.projects, projectID)
	}
	r.forgetIdleSessionsLocked()
	return true
}

func (r *logCollabRegistry) drop(conn *WsSyncConn) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var changed []string
	for projectID, entries := range r.projects {
		if entries[conn] == nil {
			continue
		}
		delete(entries, conn)
		if len(entries) == 0 {
			delete(r.projects, projectID)
		}
		changed = append(changed, projectID)
	}
	delete(r.sessions, conn)
	return changed
}

func (r *logCollabRegistry) prune() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pruneLocked(r.now())
}

func (r *logCollabRegistry) snapshot(projectID string) []logPresenceView {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(projectID)
}

func (r *logCollabRegistry) participants(projectID string) []*WsSyncConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*WsSyncConn, 0, len(r.projects[projectID]))
	for conn := range r.projects[projectID] {
		out = append(out, conn)
	}
	return out
}

func (r *logCollabRegistry) startSweeper(broadcast func(projectID string)) {
	r.sweeper.Do(func() {
		go func() {
			ticker := time.NewTicker(logPresenceSweep)
			defer ticker.Stop()
			for range ticker.C {
				for _, projectID := range r.prune() {
					broadcast(projectID)
				}
			}
		}()
	})
}

func broadcastLogPresence(projectID string, except *WsSyncConn) {
	event := fiber.Map{"op": protocol.OpEvent, "type": "log-project-presence", "projectId": projectID, "presences": logCollab.snapshot(projectID)}
	for _, conn := range logCollab.participants(projectID) {
		if conn != except {
			writeConnJSONAndPrune(nil, conn, event)
		}
	}
}

// notifyLogDraftViewers 通知正在打开工作台的连接草稿版本已推进（HTTP 写入没有来源连接，发起者也会收到，
// 客户端按版本识别自己的回声）。reason=draft 表示只改了草稿内容，客户端无需重载消息窗口。
func notifyLogDraftViewers(projectID string, version int64, reason string) {
	event := fiber.Map{"op": protocol.OpEvent, "type": "log-project-draft-changed", "projectId": projectID, "draftVersion": version, "reason": reason}
	for _, conn := range logCollab.participants(projectID) {
		writeConnJSONAndPrune(nil, conn, event)
	}
}

// Recheck access at delivery time, including for private changes on the creator's other tabs.
func (r *logCollabRegistry) bookmarkViewers(projectID, creatorID string, shared bool, canAccess func(string, string) error) []*WsSyncConn {
	r.mu.Lock()
	candidates := make(map[*WsSyncConn]string)
	for conn, entry := range r.projects[projectID] {
		if shared || entry.view.UserID == creatorID {
			candidates[conn] = entry.view.UserID
		}
	}
	r.mu.Unlock()
	var out []*WsSyncConn
	checked := make(map[string]bool)
	allowed := make(map[string]bool)
	for conn, userID := range candidates {
		if !checked[userID] {
			allowed[userID] = canAccess(projectID, userID) == nil
			checked[userID] = true
		}
		if allowed[userID] {
			out = append(out, conn)
		}
	}
	return out
}

// Invalidation only: no bookmark ID, name, position or creator is broadcast.
func notifyLogBookmarkViewers(projectID string, item *service.LogBookmark) {
	event := fiber.Map{"op": protocol.OpEvent, "type": "log-project-bookmarks-changed", "projectId": projectID}
	for _, conn := range logCollab.bookmarkViewers(projectID, item.CreatorID, item.Visibility == service.LogBookmarkShared, service.CheckLogProjectDesigner) {
		writeConnJSONAndPrune(nil, conn, event)
	}
}

func dropLogPresence(conn *WsSyncConn) {
	for _, projectID := range logCollab.drop(conn) {
		broadcastLogPresence(projectID, nil)
	}
}

func validLogPresenceID(id string, required bool) bool {
	if id == "" {
		return !required
	}
	return len(id) <= logPresenceMaxIDLength && strings.TrimSpace(id) == id
}

func apiLogPresenceUpdate(ctx *ChatContext, data logPresenceRequest) (any, error) {
	if ctx == nil || ctx.User == nil || ctx.Conn == nil || ctx.IsReadOnly() || ctx.User.IsBot {
		return nil, &service.LogError{Code: "PERMISSION_DENIED", Message: "无权参与排版协作"}
	}
	if !validLogPresenceID(data.ProjectID, true) || !validLogPresenceID(data.SectionID, false) || !validLogPresenceID(data.MessageID, false) || !validLogPresenceID(data.EditingMessageID, false) {
		return nil, &service.LogError{Code: "INVALID_INPUT", Message: "协作状态参数无效"}
	}
	logCollab.startSweeper(func(projectID string) { broadcastLogPresence(projectID, nil) })
	checked := false
	if logCollab.needsAccessCheck(ctx.Conn, data.ProjectID) {
		if err := service.CheckLogProjectDesigner(data.ProjectID, ctx.User.ID); err != nil {
			if logCollab.leave(ctx.Conn, data.ProjectID) {
				broadcastLogPresence(data.ProjectID, ctx.Conn)
			}
			var domain *service.LogError
			if errors.As(err, &domain) {
				return nil, domain
			}
			return nil, &service.LogError{Code: "INTERNAL_ERROR", Message: "LOG 操作失败"}
		}
		checked = true
	}
	name := strings.TrimSpace(ctx.User.Nickname)
	if name == "" {
		name = ctx.User.Username
	}
	result, changed, err := logCollab.apply(ctx.Conn, logPresenceIdentity{UserID: ctx.User.ID, Name: name, Avatar: ctx.User.Avatar}, data, checked)
	for _, projectID := range changed {
		broadcastLogPresence(projectID, ctx.Conn)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func apiLogPresenceLeave(ctx *ChatContext, data logPresenceRequest) (any, error) {
	if ctx == nil || ctx.Conn == nil || !validLogPresenceID(data.ProjectID, true) {
		return nil, &service.LogError{Code: "INVALID_INPUT", Message: "协作状态参数无效"}
	}
	if logCollab.leave(ctx.Conn, data.ProjectID) {
		broadcastLogPresence(data.ProjectID, ctx.Conn)
	}
	return fiber.Map{"success": true}, nil
}
