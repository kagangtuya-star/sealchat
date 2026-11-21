package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
)

// 布局频道（工作台）协议版本。存储、接口与导入导出共用同一份 layout 结构；
// 升级时在 migrateChannelWorkbenchLayout 中按版本迁移，旧版本服务端拒绝未知的新版本。
const (
	ChannelWorkbenchLayoutVersion       = 1
	ChannelWorkbenchPresentationVersion = 1

	channelWorkbenchMaxPanels      = 48
	channelWorkbenchMaxLayoutBytes = 256 * 1024
	channelWorkbenchMinRows        = 4
	channelWorkbenchMaxRows        = 60
	channelWorkbenchMaxGap         = 32
	channelWorkbenchMaxTitleRunes  = 80
	channelWorkbenchMaxLabelRunes  = 120
	channelWorkbenchMaxResourceID  = 200
	channelWorkbenchMaxChannelID   = 100
	channelWorkbenchMaxURLLength   = 2048
	channelWorkbenchCASRetries     = 4
)

var (
	ErrChannelWorkbenchNotFound           = errors.New("channel workbench: channel not found")
	ErrChannelWorkbenchDenied             = errors.New("channel workbench: permission denied")
	ErrChannelWorkbenchInvalid            = errors.New("channel workbench: invalid input")
	ErrChannelWorkbenchConflict           = errors.New("channel workbench: revision conflict")
	ErrChannelWorkbenchUnsupportedVersion = errors.New("channel workbench: unsupported layout version")
)

// ChannelWorkbenchValidationError carries a user-visible reason; errors.Is(err, ErrChannelWorkbenchInvalid) holds.
type ChannelWorkbenchValidationError struct {
	Reason string
}

func (e *ChannelWorkbenchValidationError) Error() string { return "invalid workbench: " + e.Reason }
func (e *ChannelWorkbenchValidationError) Unwrap() error { return ErrChannelWorkbenchInvalid }

func workbenchInvalid(format string, args ...any) error {
	return &ChannelWorkbenchValidationError{Reason: fmt.Sprintf(format, args...)}
}

var (
	channelWorkbenchColumns         = map[int]bool{12: true, 24: true}
	channelWorkbenchSurfaceTypes    = map[string]bool{"character": true, "iform": true, "note": true, "clue": true, "clue-board": true}
	channelWorkbenchMobileModes     = map[string]bool{"": true, "tabs": true, "stack": true}
	channelWorkbenchChromes         = map[string]bool{"": true, "default": true, "minimal": true}
	channelWorkbenchWebDisplayModes = map[string]bool{"": true, "embed": true, "link": true}
	channelWorkbenchPanelStyles     = map[string]bool{"": true, "flat": true, "outlined": true, "glass": true}
	channelWorkbenchAccentPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	channelWorkbenchPanelIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
)

type ChannelWorkbenchGrid struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
	Gap     int `json:"gap"`
}

// ChannelWorkbenchSettings 中 PanelStyle/Accent 是布局级主题；深度定制交给界面自定义包的 CSS hook。
type ChannelWorkbenchSettings struct {
	MobileMode              string `json:"mobileMode,omitempty"`
	PanelStyle              string `json:"panelStyle,omitempty"`
	MinimalActionsPlacement string `json:"minimalActionsPlacement,omitempty"`
	Accent                  string `json:"accent,omitempty"`
}

type ChannelWorkbenchRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// ChannelWorkbenchSource 是面板的资源描述，而不是生成后的 URL。
// type=surface 引用 Internal Surface 资源；type=chat 引用频道聊天（复用 Embed Chat）；
// type=theater 引用频道的小剧场舞台；type=web 是外部网页。channelId 为空表示布局频道自身。
type ChannelWorkbenchSource struct {
	Type       string `json:"type"`
	Surface    string `json:"surface,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	ChannelID  string `json:"channelId,omitempty"`
	Label      string `json:"label,omitempty"`
	URL        string `json:"url,omitempty"`
	Display    string `json:"display,omitempty"`
}

type ChannelWorkbenchPanel struct {
	ID              string                 `json:"id"`
	Title           string                 `json:"title,omitempty"`
	Rect            ChannelWorkbenchRect   `json:"rect"`
	Source          ChannelWorkbenchSource `json:"source"`
	HiddenByDefault bool                   `json:"hiddenByDefault,omitempty"`
	Chrome          string                 `json:"chrome,omitempty"`
}

type ChannelWorkbenchLayout struct {
	Version  int                      `json:"version"`
	Grid     ChannelWorkbenchGrid     `json:"grid"`
	Settings ChannelWorkbenchSettings `json:"settings"`
	Panels   []ChannelWorkbenchPanel  `json:"panels"`
}

type ChannelWorkbenchSpotlight struct {
	PanelID string `json:"panelId"`
	By      string `json:"by,omitempty"`
	At      int64  `json:"at"`
}

type ChannelWorkbenchPresentation struct {
	Version    int                        `json:"version"`
	Visibility map[string]string          `json:"visibility,omitempty"`
	Spotlight  *ChannelWorkbenchSpotlight `json:"spotlight,omitempty"`
}

type ChannelWorkbenchView struct {
	ChannelID            string                       `json:"channelId"`
	WorldID              string                       `json:"worldId"`
	DisplayMode          string                       `json:"displayMode"`
	CanManage            bool                         `json:"canManage"`
	Layout               ChannelWorkbenchLayout       `json:"layout"`
	LayoutRevision       uint64                       `json:"layoutRevision"`
	Presentation         ChannelWorkbenchPresentation `json:"presentation"`
	PresentationRevision uint64                       `json:"presentationRevision"`
	UpdatedAt            *time.Time                   `json:"updatedAt,omitempty"`
}

type ChannelWorkbenchPresentationInput struct {
	Action     string `json:"action"`
	PanelID    string `json:"panelId"`
	Visibility string `json:"visibility"`
	Reveal     bool   `json:"reveal"`
}

func DefaultChannelWorkbenchLayout() ChannelWorkbenchLayout {
	return ChannelWorkbenchLayout{
		Version: ChannelWorkbenchLayoutVersion,
		Grid:    ChannelWorkbenchGrid{Columns: 24, Rows: 12, Gap: 8},
		Panels:  []ChannelWorkbenchPanel{},
	}
}

func NormalizeChannelDisplayMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", "chat":
		return "", nil
	case model.ChannelDisplayModeWorkbench:
		return model.ChannelDisplayModeWorkbench, nil
	default:
		return "", workbenchInvalid("不支持的频道展示形态")
	}
}

func truncateRunes(value string, max int) (string, bool) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= max {
		return value, true
	}
	return value, false
}

func normalizeChannelWorkbenchURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > channelWorkbenchMaxURLLength {
		return "", workbenchInvalid("网页地址过长")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", workbenchInvalid("网页地址必须是 http(s) 绝对地址")
	}
	if parsed.User != nil {
		return "", workbenchInvalid("网页地址不能包含账号信息")
	}
	return raw, nil
}

func normalizeChannelWorkbenchSource(source ChannelWorkbenchSource) (ChannelWorkbenchSource, error) {
	out := ChannelWorkbenchSource{Type: strings.TrimSpace(source.Type)}
	label, ok := truncateRunes(source.Label, channelWorkbenchMaxLabelRunes)
	if !ok {
		return out, workbenchInvalid("资源名称过长")
	}
	channelID := strings.TrimSpace(source.ChannelID)
	if len(channelID) > channelWorkbenchMaxChannelID {
		return out, workbenchInvalid("频道 ID 无效")
	}
	switch out.Type {
	case "surface":
		surface := strings.TrimSpace(source.Surface)
		if !channelWorkbenchSurfaceTypes[surface] {
			return out, workbenchInvalid("不支持的内部面板类型：%s", surface)
		}
		resourceID := strings.TrimSpace(source.ResourceID)
		if len(resourceID) > channelWorkbenchMaxResourceID || strings.ContainsAny(resourceID, "\x00/?#") {
			return out, workbenchInvalid("资源 ID 无效")
		}
		if surface == "clue-board" && resourceID != "" && resourceID != "main" {
			return out, workbenchInvalid("线索画板资源 ID 无效")
		}
		out.Surface, out.ResourceID, out.ChannelID, out.Label = surface, resourceID, channelID, label
	case "chat", "theater":
		out.ChannelID, out.Label = channelID, label
	case "web":
		normalizedURL, err := normalizeChannelWorkbenchURL(source.URL)
		if err != nil {
			return out, err
		}
		display := strings.TrimSpace(source.Display)
		if !channelWorkbenchWebDisplayModes[display] {
			return out, workbenchInvalid("网页展示方式无效")
		}
		out.URL, out.Display = normalizedURL, display
	default:
		return out, workbenchInvalid("不支持的面板来源类型")
	}
	return out, nil
}

// normalizeChannelWorkbenchLayout validates a decoded layout and returns its canonical form.
func normalizeChannelWorkbenchLayout(layout ChannelWorkbenchLayout) (ChannelWorkbenchLayout, error) {
	if layout.Version != ChannelWorkbenchLayoutVersion {
		if layout.Version > ChannelWorkbenchLayoutVersion {
			return layout, ErrChannelWorkbenchUnsupportedVersion
		}
		return layout, workbenchInvalid("布局版本无效")
	}
	grid := layout.Grid
	if !channelWorkbenchColumns[grid.Columns] {
		return layout, workbenchInvalid("网格列数只能是 12 或 24")
	}
	if grid.Rows < channelWorkbenchMinRows || grid.Rows > channelWorkbenchMaxRows {
		return layout, workbenchInvalid("网格行数需在 %d-%d 之间", channelWorkbenchMinRows, channelWorkbenchMaxRows)
	}
	if grid.Gap < 0 || grid.Gap > channelWorkbenchMaxGap {
		return layout, workbenchInvalid("面板间距无效")
	}
	mobileMode := strings.TrimSpace(layout.Settings.MobileMode)
	if !channelWorkbenchMobileModes[mobileMode] {
		return layout, workbenchInvalid("移动端展示方式无效")
	}
	panelStyle := strings.TrimSpace(layout.Settings.PanelStyle)
	if !channelWorkbenchPanelStyles[panelStyle] {
		return layout, workbenchInvalid("面板风格无效")
	}
	minimalActionsPlacement := strings.TrimSpace(layout.Settings.MinimalActionsPlacement)
	switch minimalActionsPlacement {
	case "", "inside":
		minimalActionsPlacement = ""
	case "outside":
	default:
		return layout, workbenchInvalid("极简操作区位置无效")
	}
	accent := strings.TrimSpace(layout.Settings.Accent)
	if accent != "" && !channelWorkbenchAccentPattern.MatchString(accent) {
		return layout, workbenchInvalid("强调色必须是 #RRGGBB")
	}
	if len(layout.Panels) > channelWorkbenchMaxPanels {
		return layout, workbenchInvalid("面板数量不能超过 %d", channelWorkbenchMaxPanels)
	}
	out := ChannelWorkbenchLayout{
		Version:  ChannelWorkbenchLayoutVersion,
		Grid:     grid,
		Settings: ChannelWorkbenchSettings{MobileMode: mobileMode, PanelStyle: panelStyle, MinimalActionsPlacement: minimalActionsPlacement, Accent: strings.ToLower(accent)},
		Panels:   make([]ChannelWorkbenchPanel, 0, len(layout.Panels)),
	}
	seen := make(map[string]bool, len(layout.Panels))
	for _, panel := range layout.Panels {
		id := strings.TrimSpace(panel.ID)
		if !channelWorkbenchPanelIDPattern.MatchString(id) {
			return layout, workbenchInvalid("面板 ID 无效")
		}
		if seen[id] {
			return layout, workbenchInvalid("面板 ID 重复：%s", id)
		}
		seen[id] = true
		title, ok := truncateRunes(panel.Title, channelWorkbenchMaxTitleRunes)
		if !ok {
			return layout, workbenchInvalid("面板标题过长")
		}
		rect := panel.Rect
		if rect.X < 0 || rect.Y < 0 || rect.W < 1 || rect.H < 1 || rect.X+rect.W > grid.Columns || rect.Y+rect.H > grid.Rows {
			return layout, workbenchInvalid("面板「%s」超出网格范围", id)
		}
		for _, other := range out.Panels {
			b := other.Rect
			if rect.X < b.X+b.W && b.X < rect.X+rect.W && rect.Y < b.Y+b.H && b.Y < rect.Y+rect.H {
				return layout, workbenchInvalid("面板「%s」（%s）与「%s」（%s）发生重叠", title, id, other.Title, other.ID)
			}
		}
		chrome := strings.TrimSpace(panel.Chrome)
		if !channelWorkbenchChromes[chrome] {
			return layout, workbenchInvalid("面板外观无效")
		}
		if chrome == "default" {
			chrome = ""
		}
		source, err := normalizeChannelWorkbenchSource(panel.Source)
		if err != nil {
			return layout, err
		}
		out.Panels = append(out.Panels, ChannelWorkbenchPanel{
			ID:              id,
			Title:           title,
			Rect:            rect,
			Source:          source,
			HiddenByDefault: panel.HiddenByDefault,
			Chrome:          chrome,
		})
	}
	return out, nil
}

// migrateChannelWorkbenchLayout upgrades stored/imported documents to the current version.
// Version 1 is the first released schema, so it only needs strict decoding today.
func migrateChannelWorkbenchLayout(raw []byte) (ChannelWorkbenchLayout, error) {
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ChannelWorkbenchLayout{}, workbenchInvalid("布局不是合法 JSON")
	}
	if probe.Version > ChannelWorkbenchLayoutVersion {
		return ChannelWorkbenchLayout{}, ErrChannelWorkbenchUnsupportedVersion
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var layout ChannelWorkbenchLayout
	if err := decoder.Decode(&layout); err != nil {
		return ChannelWorkbenchLayout{}, workbenchInvalid("布局结构无效")
	}
	return normalizeChannelWorkbenchLayout(layout)
}

func ParseChannelWorkbenchLayout(raw json.RawMessage) (ChannelWorkbenchLayout, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ChannelWorkbenchLayout{}, workbenchInvalid("缺少布局")
	}
	if len(trimmed) > channelWorkbenchMaxLayoutBytes {
		return ChannelWorkbenchLayout{}, workbenchInvalid("布局数据过大")
	}
	return migrateChannelWorkbenchLayout(trimmed)
}

func decodeStoredChannelWorkbenchLayout(raw string) ChannelWorkbenchLayout {
	if strings.TrimSpace(raw) == "" {
		return DefaultChannelWorkbenchLayout()
	}
	layout, err := migrateChannelWorkbenchLayout([]byte(raw))
	if err != nil {
		return DefaultChannelWorkbenchLayout()
	}
	return layout
}

func decodeStoredChannelWorkbenchPresentation(raw string) ChannelWorkbenchPresentation {
	presentation := ChannelWorkbenchPresentation{Version: ChannelWorkbenchPresentationVersion}
	if strings.TrimSpace(raw) == "" {
		return presentation
	}
	if err := json.Unmarshal([]byte(raw), &presentation); err != nil || presentation.Version != ChannelWorkbenchPresentationVersion {
		return ChannelWorkbenchPresentation{Version: ChannelWorkbenchPresentationVersion}
	}
	return presentation
}

// prunePresentation drops overrides and spotlight that no longer reference a layout panel.
func prunePresentation(presentation ChannelWorkbenchPresentation, layout ChannelWorkbenchLayout) ChannelWorkbenchPresentation {
	panelIDs := make(map[string]bool, len(layout.Panels))
	for _, panel := range layout.Panels {
		panelIDs[panel.ID] = true
	}
	out := ChannelWorkbenchPresentation{Version: ChannelWorkbenchPresentationVersion}
	for id, value := range presentation.Visibility {
		if panelIDs[id] && (value == "shown" || value == "hidden") {
			if out.Visibility == nil {
				out.Visibility = map[string]string{}
			}
			out.Visibility[id] = value
		}
	}
	if presentation.Spotlight != nil && panelIDs[presentation.Spotlight.PanelID] {
		spotlight := *presentation.Spotlight
		out.Spotlight = &spotlight
	}
	return out
}

func channelWorkbenchPanelHidden(panel ChannelWorkbenchPanel, presentation ChannelWorkbenchPresentation) bool {
	switch presentation.Visibility[panel.ID] {
	case "hidden":
		return true
	case "shown":
		return false
	default:
		return panel.HiddenByDefault
	}
}

// filterChannelWorkbenchForViewer removes panels hidden from players and every
// override detail, so a non-manager never receives hidden panel definitions.
func filterChannelWorkbenchForViewer(layout ChannelWorkbenchLayout, presentation ChannelWorkbenchPresentation) (ChannelWorkbenchLayout, ChannelWorkbenchPresentation) {
	visible := make([]ChannelWorkbenchPanel, 0, len(layout.Panels))
	visibleIDs := map[string]bool{}
	for _, panel := range layout.Panels {
		if channelWorkbenchPanelHidden(panel, presentation) {
			continue
		}
		panel.HiddenByDefault = false
		visible = append(visible, panel)
		visibleIDs[panel.ID] = true
	}
	layout.Panels = visible
	out := ChannelWorkbenchPresentation{Version: ChannelWorkbenchPresentationVersion}
	if presentation.Spotlight != nil && visibleIDs[presentation.Spotlight.PanelID] {
		out.Spotlight = &ChannelWorkbenchSpotlight{PanelID: presentation.Spotlight.PanelID, At: presentation.Spotlight.At}
	}
	return layout, out
}

func loadChannelWorkbenchChannel(channelID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, ErrChannelWorkbenchNotFound
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" || channel.Status == "deleted" || channel.IsPrivate || strings.TrimSpace(channel.WorldID) == "" {
		return nil, ErrChannelWorkbenchNotFound
	}
	return channel, nil
}

// ChannelWorkbenchCanManage 与频道基础设置保持一致：世界 owner/admin、频道管理信息权限或平台管理员。
func ChannelWorkbenchCanManage(userID string, channel *model.ChannelModel) bool {
	if channel == nil || strings.TrimSpace(userID) == "" {
		return false
	}
	if IsWorldAdmin(channel.WorldID, userID) {
		return true
	}
	if pm.CanWithChannelRole(userID, channel.ID, pm.PermFuncChannelManageInfo) {
		return true
	}
	return pm.CanWithSystemRole(userID, pm.PermModAdmin)
}

func resolveChannelWorkbenchAccess(channelID, userID string, requireManage bool) (*model.ChannelModel, bool, error) {
	channel, err := loadChannelWorkbenchChannel(channelID)
	if err != nil {
		return nil, false, err
	}
	canManage := ChannelWorkbenchCanManage(userID, channel)
	if !canManage {
		if requireManage {
			return nil, false, ErrChannelWorkbenchDenied
		}
		if !CanReadChannelByUserId(userID, channel.ID) {
			return nil, false, ErrChannelWorkbenchDenied
		}
	}
	return channel, canManage, nil
}

func ChannelWorkbenchGetView(channelID, userID string) (*ChannelWorkbenchView, error) {
	channel, canManage, err := resolveChannelWorkbenchAccess(channelID, userID, false)
	if err != nil {
		return nil, err
	}
	record, err := model.ChannelWorkbenchGet(nil, channel.ID)
	if err != nil {
		return nil, err
	}
	view := &ChannelWorkbenchView{
		ChannelID:   channel.ID,
		WorldID:     channel.WorldID,
		DisplayMode: channel.DisplayMode,
		CanManage:   canManage,
		Layout:      DefaultChannelWorkbenchLayout(),
		Presentation: ChannelWorkbenchPresentation{
			Version: ChannelWorkbenchPresentationVersion,
		},
	}
	if record != nil {
		view.Layout = decodeStoredChannelWorkbenchLayout(record.LayoutJSON)
		view.Presentation = prunePresentation(decodeStoredChannelWorkbenchPresentation(record.PresentationJSON), view.Layout)
		view.LayoutRevision = record.LayoutRevision
		view.PresentationRevision = record.PresentationRevision
		updatedAt := record.UpdatedAt
		view.UpdatedAt = &updatedAt
	}
	if !canManage {
		view.Layout, view.Presentation = filterChannelWorkbenchForViewer(view.Layout, view.Presentation)
	}
	return view, nil
}

// ChannelWorkbenchSaveLayout publishes a design-mode layout with optimistic concurrency.
func ChannelWorkbenchSaveLayout(channelID, userID string, raw json.RawMessage, baseRevision uint64) (*model.ChannelModel, uint64, error) {
	channel, _, err := resolveChannelWorkbenchAccess(channelID, userID, true)
	if err != nil {
		return nil, 0, err
	}
	layout, err := ParseChannelWorkbenchLayout(raw)
	if err != nil {
		return nil, 0, err
	}
	encoded, err := json.Marshal(layout)
	if err != nil {
		return nil, 0, err
	}
	applied, err := model.ChannelWorkbenchLayoutCompareAndSet(nil, channel.ID, channel.WorldID, ChannelWorkbenchLayoutVersion, string(encoded), baseRevision, userID)
	if err != nil {
		return nil, 0, err
	}
	if !applied {
		return channel, 0, ErrChannelWorkbenchConflict
	}
	return channel, baseRevision + 1, nil
}

// ChannelWorkbenchCurrentLayoutRevision is used to report the winning revision after a conflict.
func ChannelWorkbenchCurrentLayoutRevision(channelID string) uint64 {
	record, err := model.ChannelWorkbenchGet(nil, channelID)
	if err != nil || record == nil {
		return 0
	}
	return record.LayoutRevision
}

func ChannelWorkbenchSetDisplayMode(channelID, userID, mode string) (*model.ChannelModel, error) {
	normalized, err := NormalizeChannelDisplayMode(mode)
	if err != nil {
		return nil, err
	}
	channel, _, err := resolveChannelWorkbenchAccess(channelID, userID, true)
	if err != nil {
		return nil, err
	}
	if channel.DisplayMode == normalized {
		return channel, nil
	}
	if err := model.ChannelDisplayModeSet(channel.ID, normalized); err != nil {
		return nil, err
	}
	channel.DisplayMode = normalized
	return channel, nil
}

func applyChannelWorkbenchPresentationInput(presentation ChannelWorkbenchPresentation, layout ChannelWorkbenchLayout, input ChannelWorkbenchPresentationInput, userID string, now time.Time) (ChannelWorkbenchPresentation, error) {
	next := prunePresentation(presentation, layout)
	panelID := strings.TrimSpace(input.PanelID)
	var panel *ChannelWorkbenchPanel
	for index := range layout.Panels {
		if layout.Panels[index].ID == panelID {
			panel = &layout.Panels[index]
			break
		}
	}
	setVisibility := func(value string) {
		if value == "" {
			delete(next.Visibility, panelID)
			return
		}
		if next.Visibility == nil {
			next.Visibility = map[string]string{}
		}
		next.Visibility[panelID] = value
	}
	switch strings.TrimSpace(input.Action) {
	case "visibility":
		if panel == nil {
			return next, workbenchInvalid("面板不存在")
		}
		switch input.Visibility {
		case "shown", "hidden":
			setVisibility(input.Visibility)
		case "default":
			setVisibility("")
		default:
			return next, workbenchInvalid("可见性无效")
		}
		if channelWorkbenchPanelHidden(*panel, next) && next.Spotlight != nil && next.Spotlight.PanelID == panelID {
			next.Spotlight = nil
		}
	case "spotlight":
		if panel == nil {
			return next, workbenchInvalid("面板不存在")
		}
		if channelWorkbenchPanelHidden(*panel, next) {
			if !input.Reveal {
				return next, workbenchInvalid("面板对玩家隐藏，需先揭示")
			}
			setVisibility("shown")
		}
		next.Spotlight = &ChannelWorkbenchSpotlight{PanelID: panelID, By: userID, At: now.UnixMilli()}
	case "clear-spotlight":
		next.Spotlight = nil
	case "reset":
		next = ChannelWorkbenchPresentation{Version: ChannelWorkbenchPresentationVersion}
	default:
		return next, workbenchInvalid("不支持的演示操作")
	}
	if len(next.Visibility) == 0 {
		next.Visibility = nil
	}
	return next, nil
}

// ChannelWorkbenchApplyPresentation mutates only the shared presentation state.
func ChannelWorkbenchApplyPresentation(channelID, userID string, input ChannelWorkbenchPresentationInput) (*model.ChannelModel, uint64, error) {
	channel, _, err := resolveChannelWorkbenchAccess(channelID, userID, true)
	if err != nil {
		return nil, 0, err
	}
	for attempt := 0; attempt < channelWorkbenchCASRetries; attempt++ {
		record, err := model.ChannelWorkbenchGet(nil, channel.ID)
		if err != nil {
			return nil, 0, err
		}
		if record == nil {
			return nil, 0, workbenchInvalid("布局尚未保存")
		}
		layout := decodeStoredChannelWorkbenchLayout(record.LayoutJSON)
		next, err := applyChannelWorkbenchPresentationInput(decodeStoredChannelWorkbenchPresentation(record.PresentationJSON), layout, input, userID, time.Now())
		if err != nil {
			return nil, 0, err
		}
		encoded, err := json.Marshal(next)
		if err != nil {
			return nil, 0, err
		}
		applied, err := model.ChannelWorkbenchPresentationCompareAndSet(nil, channel.ID, string(encoded), record.PresentationRevision, userID)
		if err != nil {
			return nil, 0, err
		}
		if applied {
			return channel, record.PresentationRevision + 1, nil
		}
	}
	return nil, 0, ErrChannelWorkbenchConflict
}

// CloneChannelWorkbenchTx copies the layout of a channel into a freshly cloned channel.
// Shared presentation state is runtime data and intentionally starts empty.
func CloneChannelWorkbenchTx(tx *gorm.DB, sourceChannelID, targetChannelID, targetWorldID, actorID string) error {
	record, err := model.ChannelWorkbenchGet(tx, sourceChannelID)
	if err != nil || record == nil || strings.TrimSpace(record.LayoutJSON) == "" {
		return err
	}
	now := time.Now()
	return tx.Create(&model.ChannelWorkbenchModel{
		ChannelID:      targetChannelID,
		WorldID:        targetWorldID,
		SchemaVersion:  record.SchemaVersion,
		LayoutJSON:     record.LayoutJSON,
		LayoutRevision: 1,
		UpdatedBy:      actorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error
}
