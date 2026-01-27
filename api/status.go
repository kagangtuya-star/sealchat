package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service/metrics"
)

type statusSummary struct {
	Timestamp             int64 `json:"timestamp"`
	ConcurrentConnections int64 `json:"concurrentConnections"`
	OnlineUsers           int64 `json:"onlineUsers"`
	MessagesPerMinute     int64 `json:"messagesPerMinute"`
	RegisteredUsers       int64 `json:"registeredUsers"`
	WorldCount            int64 `json:"worldCount"`
	ChannelCount          int64 `json:"channelCount"`
	PrivateChannelCount   int64 `json:"privateChannelCount"`
	MessageCount          int64 `json:"messageCount"`
	AttachmentCount       int64 `json:"attachmentCount"`
	AttachmentBytes       int64 `json:"attachmentBytes"`
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
	AttachmentCount       int64 `json:"attachmentCount"`
	AttachmentBytes       int64 `json:"attachmentBytes"`
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
	rangeDuration, ok := parseStatusRange(rangeParam)
	if !ok {
		return fiber.NewError(http.StatusBadRequest, "unsupported range")
	}
	end := time.Now().UnixMilli()
	start := end - rangeDuration.Milliseconds()
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
		OnlineUsers:           sample.OnlineUsers,
		MessagesPerMinute:     sample.MessagesPerMinute,
		RegisteredUsers:       sample.RegisteredUsers,
		WorldCount:            sample.WorldCount,
		ChannelCount:          sample.ChannelCount,
		PrivateChannelCount:   sample.PrivateChannelCount,
		MessageCount:          sample.MessageCount,
		AttachmentCount:       sample.AttachmentCount,
		AttachmentBytes:       sample.AttachmentBytes,
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
		AttachmentCount:       sample.AttachmentCount,
		AttachmentBytes:       sample.AttachmentBytes,
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
