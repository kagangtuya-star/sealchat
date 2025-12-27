package service

import (
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

// UnreadNotificationWorkerConfig Worker 配置
type UnreadNotificationWorkerConfig struct {
	CheckIntervalSec int
	MaxPerHour       int
	SiteURL          string
}

var (
	unreadNotificationWorkerOnce sync.Once
	emailService                 *EmailService
)

// StartUnreadNotificationWorker 启动未读消息通知 Worker
func StartUnreadNotificationWorker(cfg UnreadNotificationWorkerConfig, emailCfg utils.SMTPConfig) {
	unreadNotificationWorkerOnce.Do(func() {
		emailService = NewEmailService(emailCfg)
		if !emailService.IsConfigured() {
			log.Println("email-notification: SMTP 未配置，仅自定义 SMTP 可用")
		}
		log.Println("email-notification: Worker 启动")
		go runUnreadNotificationWorker(cfg)
	})
}

func runUnreadNotificationWorker(cfg UnreadNotificationWorkerConfig) {
	interval := cfg.CheckIntervalSec
	if interval <= 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		processUnreadNotifications(cfg)
	}
}

func processUnreadNotifications(cfg UnreadNotificationWorkerConfig) {
	// 获取所有启用的通知设置
	settings, err := model.EmailNotificationSettingsListEnabled()
	if err != nil {
		log.Printf("email-notification: 获取设置失败: %v", err)
		return
	}

	for _, setting := range settings {
		if setting == nil || strings.TrimSpace(setting.Email) == "" {
			continue
		}
		processUserChannelNotification(setting, cfg)
	}

	// 定期清理旧日志（保留 7 天）
	if time.Now().Minute() == 0 {
		_ = model.EmailNotificationLogCleanup(7)
	}
}

func processUserChannelNotification(setting *model.EmailNotificationSettingsModel, cfg UnreadNotificationWorkerConfig) {
	userID := setting.UserID
	channelID := setting.ChannelID
	delayMinutes := setting.DelayMinutes

	// 1. 检查小时推送限制
	maxPerHour := cfg.MaxPerHour
	if maxPerHour <= 0 {
		maxPerHour = 5
	}
	count, err := model.EmailNotificationLogCountInLastHour(userID)
	if err != nil {
		log.Printf("email-notification: 查询推送次数失败 user=%s: %v", userID, err)
		return
	}
	if count >= int64(maxPerHour) {
		return // 已达到小时限制
	}

	// 2. 获取用户最后阅读时间
	readRecords, err := model.ChannelReadListByUserId([]string{channelID}, userID)
	if err != nil {
		log.Printf("email-notification: 获取已读记录失败 user=%s channel=%s: %v", userID, channelID, err)
		return
	}

	var lastReadTime int64
	if len(readRecords) > 0 && readRecords[0] != nil {
		lastReadTime = readRecords[0].MessageTime
	}

	// 3. 获取用户最后一次推送时间（避免重复推送）
	lastLog, err := model.EmailNotificationLogGetLatest(userID, channelID)
	if err != nil {
		log.Printf("email-notification: 获取推送记录失败 user=%s channel=%s: %v", userID, channelID, err)
		return
	}

	var lastPushTime int64
	if lastLog != nil {
		lastPushTime = lastLog.SentAt
	}

	// 4. 计算延迟时间阈值
	delayMs := int64(delayMinutes) * 60 * 1000
	cutoffTime := time.Now().UnixMilli() - delayMs

	// 5. 查询未读消息
	unreadMessages, err := getUnreadMessagesForNotification(channelID, userID, lastReadTime, lastPushTime, cutoffTime)
	if err != nil {
		log.Printf("email-notification: 查询未读消息失败 user=%s channel=%s: %v", userID, channelID, err)
		return
	}

	if len(unreadMessages) == 0 {
		return // 没有需要通知的消息
	}

	// 6. 获取频道名称
	channelName := resolveChannelNameForEmail(channelID)
	channelURL := resolveChannelURLForEmail(channelID, cfg.SiteURL)

	// 7. 构建并发送邮件
	htmlBody := BuildUnreadDigestHTML(channelName, unreadMessages, normalizeSiteURL(cfg.SiteURL), channelURL)
	subject := "【SealChat】您有 " + formatMessageCount(len(unreadMessages)) + " 条未读消息"

	// 选择 SMTP 配置：用户自定义或全局
	var svc *EmailService
	if setting.UseCustomSMTP {
		if strings.TrimSpace(setting.SMTPHost) == "" {
			log.Printf("email-notification: 自定义 SMTP 未配置 user=%s channel=%s", userID, channelID)
			return
		}
		svc = NewEmailService(setting.ToSMTPConfig())
		if !svc.IsConfigured() {
			log.Printf("email-notification: 自定义 SMTP 配置不完整 user=%s channel=%s", userID, channelID)
			return
		}
	} else {
		svc = emailService
		if !svc.IsConfigured() {
			log.Printf("email-notification: 系统 SMTP 未配置 user=%s channel=%s", userID, channelID)
			return
		}
	}

	if err := svc.SendEmail(setting.Email, subject, htmlBody); err != nil {
		log.Printf("email-notification: 发送邮件失败 user=%s email=%s: %v", userID, setting.Email, err)
		return
	}

	// 8. 记录推送日志
	if err := model.EmailNotificationLogCreate(userID, channelID, len(unreadMessages)); err != nil {
		log.Printf("email-notification: 记录推送日志失败 user=%s channel=%s: %v", userID, channelID, err)
	}

	log.Printf("email-notification: 已发送 user=%s channel=%s messages=%d custom_smtp=%v", userID, channelID, len(unreadMessages), setting.UseCustomSMTP)
}

func getUnreadMessagesForNotification(channelID, userID string, lastReadTime, lastPushTime, cutoffTime int64) ([]MessageSummary, error) {
	db := model.GetDB()

	// 消息需满足：
	// 1. 在用户最后阅读之后
	// 2. 在上次推送之后（避免重复推送）
	// 3. 消息时间早于 cutoffTime（满足延迟要求）
	// 4. 不是用户自己发的
	var messages []model.MessageModel

	query := db.Where("channel_id = ?", channelID).
		Where("user_id != ?", userID).
		Where("created_at <= ?", time.UnixMilli(cutoffTime))

	if lastReadTime > 0 {
		query = query.Where("created_at > ?", time.UnixMilli(lastReadTime))
	}
	if lastPushTime > 0 {
		query = query.Where("created_at > ?", time.UnixMilli(lastPushTime))
	}

	if err := query.Order("created_at ASC").Limit(20).Find(&messages).Error; err != nil {
		return nil, err
	}

	result := make([]MessageSummary, 0, len(messages))
	for _, msg := range messages {
		senderName := "未知用户"
		if user := model.UserGet(msg.UserID); user != nil {
			if user.Nickname != "" {
				senderName = user.Nickname
			} else {
				senderName = user.Username
			}
		}
		result = append(result, MessageSummary{
			SenderName:  senderName,
			Content:     msg.Content,
			ChannelName: "",
			Time:        msg.CreatedAt,
		})
	}
	return result, nil
}

func resolveChannelNameForEmail(channelID string) string {
	if ch, err := model.ChannelGet(channelID); err == nil && ch != nil && strings.TrimSpace(ch.Name) != "" {
		return ch.Name
	}
	if fr, err := model.FriendRelationGetByID(channelID); err == nil && fr != nil {
		return "私聊"
	}
	return "频道"
}

func formatMessageCount(count int) string {
	if count <= 0 {
		return "0"
	}
	if count == 1 {
		return "1"
	}
	if count > 99 {
		return "99+"
	}
	return strconv.Itoa(count)
}

func normalizeSiteURL(siteURL string) string {
	trimmed := strings.TrimSpace(siteURL)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "http://" + trimmed
}

func resolveChannelURLForEmail(channelID, siteURL string) string {
	baseURL := normalizeSiteURL(siteURL)
	if baseURL == "" {
		return ""
	}
	ch, err := model.ChannelGet(channelID)
	if err != nil || ch == nil || strings.TrimSpace(ch.WorldID) == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(ch.WorldID), url.PathEscape(channelID))
}
