package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/service/metrics"
	"sealchat/utils"
)

type statusSummary struct {
	Timestamp             int64 `json:"timestamp"`
	ConcurrentConnections int64 `json:"concurrentConnections"`
	WsAuthedConnections   int64 `json:"wsAuthedConnections"`
	WsPreAuthConnections  int64 `json:"wsPreAuthConnections"`
	WsTotalConnections    int64 `json:"wsTotalConnections"`
	WsGuestConnections    int64 `json:"wsGuestConnections"`
	WsObserverConnections int64 `json:"wsObserverConnections"`
	WsAuthenticatedUsers  int64 `json:"wsAuthenticatedUsers"`
	OnlineUsers           int64 `json:"onlineUsers"`
	MessagesPerMinute     int64 `json:"messagesPerMinute"`
	RegisteredUsers       int64 `json:"registeredUsers"`
	WorldCount            int64 `json:"worldCount"`
	ChannelCount          int64 `json:"channelCount"`
	PrivateChannelCount   int64 `json:"privateChannelCount"`
	MessageCount          int64 `json:"messageCount"`
	MessageCountIC        int64 `json:"messageCountIc"`
	MessageCountOOC       int64 `json:"messageCountOoc"`
	MessageCharCount      int64 `json:"messageCharCount"`
	MessageCharCountIC    int64 `json:"messageCharCountIc"`
	MessageCharCountOOC   int64 `json:"messageCharCountOoc"`
	AttachmentCount       int64 `json:"attachmentCount"`
	AttachmentBytes       int64 `json:"attachmentBytes"`
	AttachmentImageCount  int64 `json:"attachmentImageCount"`
	AttachmentImageBytes  int64 `json:"attachmentImageBytes"`
	AttachmentFontCount   int64 `json:"attachmentFontCount"`
	AttachmentFontBytes   int64 `json:"attachmentFontBytes"`
	IntervalSeconds       int   `json:"intervalSeconds"`
	RetentionDays         int   `json:"retentionDays"`
}

type statusHistoryResponse struct {
	Range    string        `json:"range"`
	Interval string        `json:"interval"`
	Points   []statusPoint `json:"points"`
}

type statusPoint struct {
	Timestamp             int64 `json:"timestamp"`
	ConcurrentConnections int64 `json:"concurrentConnections"`
	OnlineUsers           int64 `json:"onlineUsers"`
	MessagesPerMinute     int64 `json:"messagesPerMinute"`
	RegisteredUsers       int64 `json:"registeredUsers"`
	WorldCount            int64 `json:"worldCount"`
	ChannelCount          int64 `json:"channelCount"`
	PrivateChannelCount   int64 `json:"privateChannelCount"`
	MessageCount          int64 `json:"messageCount"`
	MessageCharCount      int64 `json:"messageCharCount"`
	MessageCharCountIC    int64 `json:"messageCharCountIc"`
	MessageCharCountOOC   int64 `json:"messageCharCountOoc"`
	AttachmentCount       int64 `json:"attachmentCount"`
	AttachmentBytes       int64 `json:"attachmentBytes"`
	AttachmentImageCount  int64 `json:"attachmentImageCount"`
	AttachmentImageBytes  int64 `json:"attachmentImageBytes"`
	AttachmentFontCount   int64 `json:"attachmentFontCount"`
	AttachmentFontBytes   int64 `json:"attachmentFontBytes"`
}

var (
	statusCache struct {
		mu      sync.Mutex
		item    statusSummary
		expires time.Time
	}
)

// StatusLatest 返回最近一次采样结果。
func StatusLatest(c *fiber.Ctx) error {
	now := time.Now()
	statusCache.mu.Lock()
	cached := statusCache.item
	if now.Before(statusCache.expires) && cached.Timestamp != 0 {
		statusCache.mu.Unlock()
		return c.Status(http.StatusOK).JSON(cached)
	}
	statusCache.mu.Unlock()

	sample, err := latestSample()
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}
	collector := metrics.Get()
	resp := buildSummary(sample, collector)

	statusCache.mu.Lock()
	statusCache.item = resp
	statusCache.expires = time.Now().Add(5 * time.Second)
	statusCache.mu.Unlock()

	return c.Status(http.StatusOK).JSON(resp)
}

// StatusHistory 返回指定时间范围内的历史采样点。
func StatusHistory(c *fiber.Ctx) error {
	rangeParam := strings.ToLower(c.Query("range", "1h"))
	intervalParam := strings.ToLower(c.Query("interval", "1m"))
	if intervalParam != "1m" && intervalParam != "" {
		return fiber.NewError(http.StatusBadRequest, "interval only supports 1m")
	}

	startParam := strings.TrimSpace(c.Query("start"))
	endParam := strings.TrimSpace(c.Query("end"))
	var (
		start int64
		end   int64
	)

	if startParam != "" || endParam != "" {
		if startParam == "" || endParam == "" {
			return fiber.NewError(http.StatusBadRequest, "start and end must both be provided")
		}
		parsedStart, err := strconv.ParseInt(startParam, 10, 64)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid start")
		}
		parsedEnd, err := strconv.ParseInt(endParam, 10, 64)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid end")
		}
		if parsedStart <= 0 || parsedEnd <= 0 || parsedStart >= parsedEnd {
			return fiber.NewError(http.StatusBadRequest, "invalid custom range")
		}
		start = parsedStart
		end = parsedEnd
		rangeParam = "custom"
	} else {
		rangeDuration, ok := parseStatusRange(rangeParam)
		if !ok {
			return fiber.NewError(http.StatusBadRequest, "unsupported range")
		}
		end = time.Now().UnixMilli()
		start = end - rangeDuration.Milliseconds()
	}
	samples, err := model.QueryServiceMetricSamples(start, end)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}
	points := make([]statusPoint, 0, len(samples))
	for i := range samples {
		points = append(points, sampleToPoint(&samples[i]))
	}
	resp := statusHistoryResponse{
		Range:    rangeParam,
		Interval: "1m",
		Points:   points,
	}
	return c.Status(http.StatusOK).JSON(resp)
}

func latestSample() (*model.ServiceMetricSample, error) {
	if collector := metrics.Get(); collector != nil {
		if sample, ok := collector.LatestSample(); ok {
			return sample, nil
		}
	}
	sample, err := model.GetLatestServiceMetricSample()
	if err != nil {
		return nil, err
	}
	if sample == nil {
		return &model.ServiceMetricSample{TimestampMs: time.Now().UnixMilli()}, nil
	}
	return sample, nil
}

func buildSummary(sample *model.ServiceMetricSample, collector *metrics.Collector) statusSummary {
	if sample == nil {
		sample = &model.ServiceMetricSample{TimestampMs: time.Now().UnixMilli()}
	}
	attachmentCount := sample.AttachmentCount
	attachmentBytes := sample.AttachmentBytes
	attachmentImageCount := sample.AttachmentImageCount
	attachmentImageBytes := sample.AttachmentImageBytes
	attachmentFontCount := sample.AttachmentFontCount
	attachmentFontBytes := sample.AttachmentFontBytes
	if attachmentStats, err := metrics.LoadAttachmentStatusStats(time.Now()); err == nil && attachmentStats != nil {
		attachmentCount = attachmentStats.TotalCount
		attachmentBytes = attachmentStats.TotalBytes
		attachmentImageCount = attachmentStats.ImageCount
		attachmentImageBytes = attachmentStats.ImageBytes
		attachmentFontCount = attachmentStats.FontCount
		attachmentFontBytes = attachmentStats.FontBytes
	}
	wsSnapshot := getWsConnectionSnapshot()
	intervalSeconds := 120
	retentionDays := 7
	if collector != nil {
		if sec := int(collector.Interval().Seconds()); sec > 0 {
			intervalSeconds = sec
		}
		if days := int(collector.Retention().Hours() / 24); days > 0 {
			retentionDays = days
		}
	}
	return statusSummary{
		Timestamp:             sample.TimestampMs,
		ConcurrentConnections: sample.ConcurrentConnections,
		WsAuthedConnections:   wsSnapshot.AuthedConnections,
		WsPreAuthConnections:  wsSnapshot.PreAuthConnections,
		WsTotalConnections:    wsSnapshot.TotalConnections,
		WsGuestConnections:    wsSnapshot.GuestConnections,
		WsObserverConnections: wsSnapshot.ObserverConnections,
		WsAuthenticatedUsers:  wsSnapshot.AuthenticatedUsers,
		OnlineUsers:           sample.OnlineUsers,
		MessagesPerMinute:     sample.MessagesPerMinute,
		RegisteredUsers:       sample.RegisteredUsers,
		WorldCount:            sample.WorldCount,
		ChannelCount:          sample.ChannelCount,
		PrivateChannelCount:   sample.PrivateChannelCount,
		MessageCount:          sample.MessageCount,
		MessageCountIC:        sample.MessageCountIC,
		MessageCountOOC:       sample.MessageCountOOC,
		MessageCharCount:      sample.MessageCharCount,
		MessageCharCountIC:    sample.MessageCharCountIC,
		MessageCharCountOOC:   sample.MessageCharCountOOC,
		AttachmentCount:       attachmentCount,
		AttachmentBytes:       attachmentBytes,
		AttachmentImageCount:  attachmentImageCount,
		AttachmentImageBytes:  attachmentImageBytes,
		AttachmentFontCount:   attachmentFontCount,
		AttachmentFontBytes:   attachmentFontBytes,
		IntervalSeconds:       intervalSeconds,
		RetentionDays:         retentionDays,
	}
}

func sampleToPoint(sample *model.ServiceMetricSample) statusPoint {
	if sample == nil {
		sample = &model.ServiceMetricSample{TimestampMs: time.Now().UnixMilli()}
	}
	return statusPoint{
		Timestamp:             sample.TimestampMs,
		ConcurrentConnections: sample.ConcurrentConnections,
		OnlineUsers:           sample.OnlineUsers,
		MessagesPerMinute:     sample.MessagesPerMinute,
		RegisteredUsers:       sample.RegisteredUsers,
		WorldCount:            sample.WorldCount,
		ChannelCount:          sample.ChannelCount,
		PrivateChannelCount:   sample.PrivateChannelCount,
		MessageCount:          sample.MessageCount,
		MessageCharCount:      sample.MessageCharCount,
		MessageCharCountIC:    sample.MessageCharCountIC,
		MessageCharCountOOC:   sample.MessageCharCountOOC,
		AttachmentCount:       sample.AttachmentCount,
		AttachmentBytes:       sample.AttachmentBytes,
		AttachmentImageCount:  sample.AttachmentImageCount,
		AttachmentImageBytes:  sample.AttachmentImageBytes,
		AttachmentFontCount:   sample.AttachmentFontCount,
		AttachmentFontBytes:   sample.AttachmentFontBytes,
	}
}

func parseStatusRange(v string) (time.Duration, bool) {
	switch v {
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h", "1d":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}

// StatusAccessMiddleware 按平台设置限制平台状态监测的访问；默认对登录用户公开。
func StatusAccessMiddleware(c *fiber.Ctx) error {
	cfg := utils.GetConfig()
	if cfg != nil && cfg.StatusPage.AdminOnly && !pm.CanWithSystemRole(getCurUser(c).ID, pm.PermModAdmin) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "平台状态监测仅对平台管理员开放"})
	}
	return c.Next()
}

type adminWorldStatusItem struct {
	service.AdminWorldSummary
	OnlineCount int `json:"onlineCount"`
}

// AdminWorldStatusList 分页返回正常状态世界（含私有世界）的概要信息，仅平台管理员可用。
func AdminWorldStatusList(c *fiber.Ctx) error {
	page, err := service.AdminListWorldSummaries(service.AdminWorldSummaryQuery{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("pageSize", 20),
		Keyword:  c.Query("keyword"),
	})
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}
	online := countOnlineUsersByWorld()
	items := make([]adminWorldStatusItem, 0, len(page.Items))
	for _, summary := range page.Items {
		items = append(items, adminWorldStatusItem{
			AdminWorldSummary: summary,
			OnlineCount:       online[summary.ID],
		})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"items":    items,
		"total":    page.Total,
		"page":     page.Page,
		"pageSize": page.PageSize,
	})
}
