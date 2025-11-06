package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

type chatExportTestRequest struct {
	ChannelID       string   `json:"channel_id"`
	Format          string   `json:"format"`
	TimeRange       []int64  `json:"time_range"`
	IncludeOOC      bool     `json:"include_ooc"`
	IncludeArchived bool     `json:"include_archived"`
	Users           []string `json:"users"`
}

type chatExportTestResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	RequestedAt int64  `json:"requested_at"`
}

func validateExportChannel(userID, channelID string) error {
	if channelID == "" {
		return fmt.Errorf("channel_id 不能为空")
	}
	if len(channelID) < 30 {
		if !pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelReadAll) {
			return fmt.Errorf("无权限导出该频道")
		}
		return nil
	}

	fr, _ := model.FriendRelationGetByID(channelID)
	if fr.ID == "" {
		return fmt.Errorf("频道不存在")
	}
	if fr.UserID1 != userID && fr.UserID2 != userID {
		return fmt.Errorf("无权限导出该频道")
	}
	return nil
}

func execChatExportTest(userID string, req *chatExportTestRequest) (*chatExportTestResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	if err := validateExportChannel(userID, strings.TrimSpace(req.ChannelID)); err != nil {
		return nil, err
	}
	if req.Format == "" {
		req.Format = "txt"
	}
	return &chatExportTestResponse{
		TaskID:      "test-export",
		Status:      "stub",
		Message:     "导出功能尚在开发，仅供联调。",
		RequestedAt: time.Now().UnixMilli(),
	}, nil
}

func ChatExportTest(c *fiber.Ctx) error {
	var req chatExportTestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求体解析失败"})
	}
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	resp, err := execChatExportTest(user.ID, &req)
	if err != nil {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

func apiChatExportTest(ctx *ChatContext, req *chatExportTestRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, fmt.Errorf("未认证")
	}
	resp, err := execChatExportTest(ctx.User.ID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
