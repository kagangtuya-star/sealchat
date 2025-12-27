package service

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"sealchat/utils"
)

// EmailService 邮件发送服务
type EmailService struct {
	cfg utils.SMTPConfig
}

// NewEmailService 创建邮件服务实例
func NewEmailService(cfg utils.SMTPConfig) *EmailService {
	return &EmailService{cfg: cfg}
}

// IsConfigured 检查 SMTP 是否已配置
func (s *EmailService) IsConfigured() bool {
	return strings.TrimSpace(s.cfg.Host) != "" &&
		s.cfg.Port > 0 &&
		strings.TrimSpace(s.cfg.FromAddress) != ""
}

// SendEmail 发送邮件
func (s *EmailService) SendEmail(to, subject, htmlBody string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("SMTP 未配置")
	}

	from := s.cfg.FromAddress
	fromName := s.cfg.FromName
	if fromName == "" {
		fromName = "SealChat"
	}

	// 构建邮件头
	headers := map[string]string{
		"From":         fmt.Sprintf("%s <%s>", fromName, from),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
		"Date":         time.Now().Format(time.RFC1123Z),
	}

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	// 根据配置选择 TLS 或普通连接
	if s.cfg.UseTLS {
		return s.sendWithTLS(addr, to, msg.String())
	}
	return s.sendPlain(addr, to, msg.String())
}

func (s *EmailService) sendPlain(addr, to, msg string) error {
	var auth smtp.Auth
	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	return smtp.SendMail(addr, auth, s.cfg.FromAddress, []string{to}, []byte(msg))
}

func (s *EmailService) sendWithTLS(addr, to, msg string) error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: s.cfg.SkipVerify,
		ServerName:         s.cfg.Host,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS 连接失败: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}

	if err := client.Mail(s.cfg.FromAddress); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("开始发送数据失败: %w", err)
	}

	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("发送邮件内容失败: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("关闭数据写入失败: %w", err)
	}

	return client.Quit()
}

// MessageSummary 消息摘要（用于邮件内容）
type MessageSummary struct {
	SenderName  string
	Content     string
	ChannelName string
	Time        time.Time
}

// BuildUnreadDigestHTML 构建未读消息摘要 HTML
func BuildUnreadDigestHTML(channelName string, messages []MessageSummary, siteURL string, channelURL string) string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; padding: 20px; }
.container { max-width: 600px; margin: 0 auto; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
.header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: #fff; padding: 24px; text-align: center; }
.header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.header p { margin: 8px 0 0; opacity: 0.9; font-size: 14px; }
.content { padding: 24px; }
.message { padding: 16px; margin-bottom: 12px; background: #f9f9f9; border-radius: 6px; border-left: 4px solid #667eea; }
.message-header { display: flex; justify-content: space-between; margin-bottom: 8px; font-size: 12px; color: #666; }
.sender { font-weight: 600; color: #333; }
.time { color: #999; }
.message-content { color: #333; line-height: 1.6; word-break: break-word; }
.footer { padding: 16px 24px; background: #f9f9f9; text-align: center; font-size: 12px; color: #999; }
.btn { display: inline-block; padding: 12px 24px; background: #667eea; color: #fff; text-decoration: none; border-radius: 6px; margin-top: 16px; }
</style>
</head>
<body>
<div class="container">
<div class="header">
<h1>📬 您有未读消息</h1>
<p>`)
	sb.WriteString("来自频道: ")
	sb.WriteString(escapeHTML(channelName))
	sb.WriteString(`</p>
</div>
<div class="content">
<p style="color:#666;margin-bottom:16px;">以下是您尚未阅读的消息：</p>
`)

	for _, m := range messages {
		sb.WriteString(`<div class="message">
<div class="message-header">
<span class="sender">`)
		sb.WriteString(escapeHTML(m.SenderName))
		sb.WriteString(`</span>
<span class="time">`)
		sb.WriteString(m.Time.Format("2006-01-02 15:04"))
		sb.WriteString(`</span>
</div>
<div class="message-content">`)
		sb.WriteString(escapeHTML(truncateContent(m.Content, 500)))
		sb.WriteString(`</div>
</div>
`)
	}

	if channelURL != "" || siteURL != "" {
		sb.WriteString(`<div style="text-align:center;">`)
		if channelURL != "" {
			sb.WriteString(fmt.Sprintf(`<a href="%s" class="btn">进入频道</a>`, escapeHTML(channelURL)))
		}
		if siteURL != "" && siteURL != channelURL {
			sb.WriteString(fmt.Sprintf(`<a href="%s" class="btn" style="margin-left: 8px;">打开 SealChat</a>`, escapeHTML(siteURL)))
		}
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`
</div>
<div class="footer">
此邮件由 SealChat 自动发送，请勿直接回复。<br>
如需取消订阅，请在 SealChat 中关闭邮件提醒功能。
</div>
</div>
</body>
</html>`)

	return sb.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func truncateContent(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
