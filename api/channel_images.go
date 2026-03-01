package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/model"
)

// channelImageItem represents a single image from channel messages
type channelImageItem struct {
	ID           string `json:"id"`
	MessageID    string `json:"message_id"`
	AttachmentID string `json:"attachment_id"`
	ThumbURL     string `json:"thumb_url"`
	SenderID     string `json:"sender_id"`
	SenderName   string `json:"sender_name"`
	SenderAvatar string `json:"sender_avatar"`
	CreatedAt    int64  `json:"created_at"`
	DisplayOrder float64 `json:"display_order"`
}

type channelImagesResponse struct {
	Items    []channelImageItem `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	HasMore  bool               `json:"has_more"`
}

// Regex patterns for extracting image attachment IDs from message content
var (
	// Match id:xxx format (e.g., id:abc123)
	attachmentIDPattern = regexp.MustCompile(`id:([a-zA-Z0-9_-]+)`)
	// Match <img> tags with src="id:xxx" or src='id:xxx'
	imgSrcPattern = regexp.MustCompile(`<img[^>]+src=["']id:([a-zA-Z0-9_-]+)["'][^>]*>`)
	// Match image elements like <image ... id:xxx/>
	satoriImagePattern = regexp.MustCompile(`<image[^>]+src=["']id:([a-zA-Z0-9_-]+)["'][^>]*>`)
)

// ChannelImagesList returns paginated images from channel messages
func ChannelImagesList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少频道ID",
		})
	}

	// Check channel access
	_, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		if err == fiber.ErrForbidden {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		}
		if err == fiber.ErrNotFound {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", 50)
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	db := model.GetDB()

	// Build base query for messages with images
	// We look for messages that likely contain images (have id: pattern in content)
	baseQuery := func() *gorm.DB {
		q := db.Model(&model.MessageModel{}).
			Where("channel_id = ?", channelID).
			Where("is_revoked = ?", false).
			Where("is_deleted = ?", false).
			Where("content LIKE ?", "%id:%") // Only get messages that might have images
		q = applyWhisperVisibilityFilter(q, user.ID, channelID)
		return q
	}

	// Count total
	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "查询失败",
		})
	}

	// Fetch messages
	offset := (page - 1) * pageSize
	var messages []*model.MessageModel
	err = baseQuery().
		Order("display_order DESC").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize * 3). // Fetch more since not all messages will have images
		Preload("User", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id, username, nickname, avatar, is_bot")
		}).
		Find(&messages).Error
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "查询失败",
		})
	}

	// Extract images from messages
	var items []channelImageItem
	seen := make(map[string]bool)

	for _, msg := range messages {
		if len(items) >= pageSize {
			break
		}

		attachmentIDs := extractImageAttachmentIDs(msg.Content)
		if len(attachmentIDs) == 0 {
			continue
		}

		senderName := resolveSenderName(msg)
		senderAvatar := ""
		senderID := ""
		if msg.User != nil {
			senderID = msg.User.ID
			senderAvatar = msg.User.Avatar
		}

		for _, attID := range attachmentIDs {
			if seen[attID] {
				continue
			}
			seen[attID] = true

			if len(items) >= pageSize {
				break
			}

			items = append(items, channelImageItem{
				ID:           msg.ID + "_" + attID,
				MessageID:    msg.ID,
				AttachmentID: attID,
				ThumbURL:     "/api/v1/attachment/" + attID + "/thumb?size=150",
				SenderID:     senderID,
				SenderName:   senderName,
				SenderAvatar: senderAvatar,
				CreatedAt:    msg.CreatedAt.UnixMilli(),
				DisplayOrder: msg.DisplayOrder,
			})
		}
	}

	resp := channelImagesResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		HasMore:  int64(page*pageSize) < total,
	}

	return c.JSON(resp)
}

// extractImageAttachmentIDs extracts attachment IDs from message content
func extractImageAttachmentIDs(content string) []string {
	if content == "" {
		return nil
	}

	var ids []string
	seen := make(map[string]bool)

	// Extract from <img src="id:xxx"> format
	matches := imgSrcPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			ids = append(ids, match[1])
			seen[match[1]] = true
		}
	}

	// Extract from <image src="id:xxx"> format (Satori)
	matches = satoriImagePattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			ids = append(ids, match[1])
			seen[match[1]] = true
		}
	}

	// Fallback: extract from id:xxx pattern if no structured format found
	if len(ids) == 0 {
		matches = attachmentIDPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 && !seen[match[1]] {
				ids = append(ids, match[1])
				seen[match[1]] = true
			}
		}
	}

	return lo.Uniq(ids)
}
