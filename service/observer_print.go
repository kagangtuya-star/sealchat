package service

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"time"

	"sealchat/model"
)

type ObserverPrintOptions struct {
	MessageScope  int
	ShowArchived  bool
	ShowTimestamp bool
	ShowColorCode bool
}

type ObserverPrintMessage struct {
	ID          string
	CreatedAt   time.Time
	SenderName  string
	SenderColor string
	Body        string
	IcMode      string
	IsArchived  bool
}

type ObserverPrintPageData struct {
	Slug             string
	WorldID          string
	WorldName        string
	ChannelID        string
	ChannelName      string
	GeneratedAt      time.Time
	MessageScope     int
	MessageScopeText string
	ShowArchived     bool
	ShowTimestamp    bool
	ShowColorCode    bool
	ArchivedText     string
	Count            int
	Messages         []ObserverPrintMessage
}

func LoadObserverPrintableMessages(channelID string, opts ObserverPrintOptions) ([]*model.MessageModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("channel id required")
	}

	q := model.GetDB().Model(&model.MessageModel{}).
		Where("channel_id = ?", channelID).
		Where("is_revoked = ?", false).
		Where("is_deleted = ?", false).
		Where("is_whisper = ?", false).
		Preload("User").
		Preload("Member")

	switch opts.MessageScope {
	case 1:
		q = q.Where("ic_mode = ?", "ooc")
	case 2:
		q = q.Where("COALESCE(ic_mode, 'ic') = ?", "ic")
	}

	if !opts.ShowArchived {
		q = q.Where("is_archived = ?", false)
	}

	q = q.Order("display_order asc").Order("created_at asc")

	var messages []*model.MessageModel
	if err := q.Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

func BuildObserverPrintPageData(world *model.WorldModel, channel *model.ChannelModel, slug string, messages []*model.MessageModel, opts ObserverPrintOptions) *ObserverPrintPageData {
	channelName := ""
	channelID := ""
	if channel != nil {
		channelName = strings.TrimSpace(channel.Name)
		channelID = strings.TrimSpace(channel.ID)
	}
	worldName := ""
	worldID := ""
	if world != nil {
		worldName = strings.TrimSpace(world.Name)
		worldID = strings.TrimSpace(world.ID)
	}

	job := &model.MessageExportJobModel{
		ChannelID:        channelID,
		IncludeOOC:       opts.MessageScope != 2,
		IncludeArchived:  opts.ShowArchived,
		WithoutTimestamp: !opts.ShowTimestamp,
		MergeMessages:    false,
	}
	payload := buildExportPayload(job, channelName, messages, nil, &exportExtraOptions{
		IncludeImages:      false,
		IncludeDiceCommand: true,
	})

	items := make([]ObserverPrintMessage, 0, len(payload.Messages))
	for i := range payload.Messages {
		msg := payload.Messages[i]
		items = append(items, ObserverPrintMessage{
			ID:          msg.ID,
			CreatedAt:   msg.CreatedAt,
			SenderName:  strings.TrimSpace(msg.SenderName),
			SenderColor: strings.TrimSpace(msg.SenderColor),
			Body:        buildContentBody(&msg, false),
			IcMode:      fallbackIcMode(msg.IcMode),
			IsArchived:  msg.IsArchived,
		})
	}

	return &ObserverPrintPageData{
		Slug:             strings.TrimSpace(slug),
		WorldID:          worldID,
		WorldName:        worldName,
		ChannelID:        channelID,
		ChannelName:      channelName,
		GeneratedAt:      payload.GeneratedAt,
		MessageScope:     opts.MessageScope,
		MessageScopeText: observerPrintMessageScopeText(opts.MessageScope),
		ShowArchived:     opts.ShowArchived,
		ShowTimestamp:    opts.ShowTimestamp,
		ShowColorCode:    opts.ShowColorCode,
		ArchivedText:     observerPrintArchivedText(opts.ShowArchived),
		Count:            len(items),
		Messages:         items,
	}
}

func RenderObserverPrintHTML(data *ObserverPrintPageData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("observer print data is nil")
	}
	var buf bytes.Buffer
	if err := observerPrintTemplate.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func RenderObserverPrintErrorHTML(title, message string) ([]byte, error) {
	var buf bytes.Buffer
	if err := observerPrintErrorTemplate.Execute(&buf, map[string]string{
		"Title":   strings.TrimSpace(title),
		"Message": strings.TrimSpace(message),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func observerPrintMessageScopeText(scope int) string {
	switch scope {
	case 1:
		return "仅场外"
	case 2:
		return "仅场内"
	default:
		return "全部消息"
	}
}

func observerPrintArchivedText(showArchived bool) string {
	if showArchived {
		return "显示归档"
	}
	return "隐藏归档"
}

var observerPrintTemplate = htmltemplate.Must(htmltemplate.New("observer_print").Funcs(htmltemplate.FuncMap{
	"formatTime": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="robots" content="noindex,nofollow">
  <title>{{.WorldName}} / {{.ChannelName}} - OB 打印页</title>
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", sans-serif;
    }
    body {
      margin: 0;
      background: #f7f8fa;
      color: #1f2937;
      line-height: 1.6;
    }
    .page {
      max-width: 1040px;
      margin: 0 auto;
      padding: 24px 20px 40px;
    }
    .header {
      background: #ffffff;
      border: 1px solid #e5e7eb;
      border-radius: 12px;
      padding: 20px;
      margin-bottom: 20px;
    }
    .title {
      margin: 0 0 8px;
      font-size: 28px;
      line-height: 1.25;
    }
    .subtitle {
      margin: 0;
      color: #6b7280;
      word-break: break-all;
    }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 14px;
    }
    .chip {
      display: inline-flex;
      align-items: center;
      padding: 4px 10px;
      border-radius: 999px;
      background: #eef2ff;
      color: #3730a3;
      font-size: 13px;
    }
    .summary {
      margin-top: 14px;
      color: #4b5563;
      font-size: 14px;
    }
    .message-list {
      display: grid;
      gap: 12px;
    }
    .message {
      background: #ffffff;
      border: 1px solid #e5e7eb;
      border-radius: 12px;
      padding: 16px;
    }
    .message-header {
      display: flex;
      flex-wrap: wrap;
      gap: 8px 12px;
      margin-bottom: 8px;
      color: #6b7280;
      font-size: 14px;
    }
    .sender {
      color: #111827;
      font-weight: 600;
    }
    .body {
      white-space: pre-wrap;
      word-break: break-word;
    }
    .empty {
      background: #ffffff;
      border: 1px dashed #d1d5db;
      border-radius: 12px;
      padding: 24px;
      text-align: center;
      color: #6b7280;
    }
  </style>
</head>
<body>
  <div class="page">
    <header class="header">
      <h1 class="title">{{.WorldName}} / {{.ChannelName}}</h1>
      <p class="subtitle">OB Slug: {{.Slug}} · World ID: {{.WorldID}} · Channel ID: {{.ChannelID}}</p>
      <div class="meta">
        <span class="chip">{{.MessageScopeText}}</span>
        <span class="chip">{{.ArchivedText}}</span>
        <span class="chip">消息数 {{.Count}}</span>
      </div>
      <div class="summary">生成时间：{{formatTime .GeneratedAt}}</div>
    </header>
    {{if .Messages}}
    <main class="message-list">
      {{range .Messages}}
      <article class="message" data-message-id="{{.ID}}" data-ic-mode="{{.IcMode}}" data-archived="{{if .IsArchived}}1{{else}}0{{end}}">
        <div class="message-header">
          {{if $.ShowTimestamp}}<span>{{formatTime .CreatedAt}}</span>{{end}}
          <span class="sender">{{.SenderName}}</span>
          {{if and $.ShowColorCode .SenderColor}}<span>{{.SenderColor}}</span>{{end}}
        </div>
        <div class="body">{{.Body}}</div>
      </article>
      {{end}}
    </main>
    {{else}}
    <div class="empty">当前筛选条件下暂无可显示消息。</div>
    {{end}}
  </div>
</body>
</html>`))

var observerPrintErrorTemplate = htmltemplate.Must(htmltemplate.New("observer_print_error").Parse(`<!DOCTYPE html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="robots" content="noindex,nofollow">
  <title>{{.Title}}</title>
  <style>
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      background: #f7f8fa;
      color: #1f2937;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", sans-serif;
    }
    .card {
      width: min(560px, calc(100vw - 32px));
      background: #ffffff;
      border: 1px solid #e5e7eb;
      border-radius: 14px;
      padding: 24px;
      box-sizing: border-box;
    }
    h1 {
      margin: 0 0 12px;
      font-size: 24px;
    }
    p {
      margin: 0;
      color: #4b5563;
      line-height: 1.7;
    }
  </style>
</head>
<body>
  <section class="card">
    <h1>{{.Title}}</h1>
    <p>{{.Message}}</p>
  </section>
</body>
</html>`))
