package metrics

import (
	"context"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

// Config 定义采样周期、保留时间等行为。
type Config struct {
	Interval  time.Duration
	Retention time.Duration
	OnlineTTL time.Duration
}

// Collector 负责汇聚实时指标并周期性写入数据库。
type Collector struct {
	cfg           Config
	connCount     atomic.Int64
	messageWindow atomic.Int64
	userStates    sync.Map // string -> *userPresence
	latestSample  atomic.Pointer[model.ServiceMetricSample]
}

type userPresence struct {
	connections atomic.Int32
	lastActive  atomic.Int64
}

var (
	defaultCollector *Collector
	initOnce         sync.Once
)

// Init 创建全局采集器实例。
func Init(cfg Config) *Collector {
	initOnce.Do(func() {
		defaultCollector = newCollector(cfg)
	})
	return defaultCollector
}

// Get 返回全局采集器。
func Get() *Collector {
	return defaultCollector
}

func newCollector(cfg Config) *Collector {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 7 * 24 * time.Hour
	}
	if cfg.OnlineTTL <= 0 {
		cfg.OnlineTTL = 2 * time.Minute
	}
	return &Collector{cfg: cfg}
}

// Start 启动后台采样循环。
func (c *Collector) Start(ctx context.Context) {
	if c == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.warmLatestFromDB()
	c.sampleOnce()
	go c.loop(ctx)
}

func (c *Collector) loop(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.sampleOnce()
		case <-ctx.Done():
			return
		}
	}
}

func (c *Collector) sampleOnce() {
	sample, err := c.buildSample()
	if err != nil {
		log.Printf("metrics: build sample failed: %v", err)
		return
	}
	c.latestSample.Store(sample)
	if err := model.InsertServiceMetricSample(sample); err != nil {
		log.Printf("metrics: insert sample failed: %v", err)
	}
	cutoff := sample.TimestampMs - c.cfg.Retention.Milliseconds()
	if err := model.DeleteServiceMetricBefore(cutoff); err != nil {
		log.Printf("metrics: cleanup expired samples failed: %v", err)
	}
}

func (c *Collector) warmLatestFromDB() {
	if c == nil {
		return
	}
	sample, err := model.GetLatestServiceMetricSample()
	if err != nil || sample == nil {
		return
	}
	c.latestSample.Store(sample)
}

func (c *Collector) buildSample() (*model.ServiceMetricSample, error) {
	now := time.Now()
	timestamp := now.UnixMilli()
	conn := c.connCount.Load()
	online := c.countOnlineUsers(timestamp)
	window := c.messageWindow.Swap(0)
	messagesPerMinute := c.normalizeThroughput(window)

	userTotal, err := model.CountActiveUsers()
	if err != nil {
		return nil, err
	}
	worldCount, err := model.CountWorlds()
	if err != nil {
		return nil, err
	}
	channelCount, err := model.CountChannels()
	if err != nil {
		return nil, err
	}
	privateChannelCount, err := model.CountPrivateChannels()
	if err != nil {
		return nil, err
	}
	messageCount, err := model.CountMessages()
	if err != nil {
		return nil, err
	}
	attachmentCount, attachmentBytes := getAttachmentDiskStats()

	sample := &model.ServiceMetricSample{
		TimestampMs:           timestamp,
		ConcurrentConnections: conn,
		OnlineUsers:           online,
		MessagesPerMinute:     messagesPerMinute,
		RegisteredUsers:       userTotal,
		WorldCount:            worldCount,
		ChannelCount:          channelCount,
		PrivateChannelCount:   privateChannelCount,
		MessageCount:          messageCount,
		AttachmentCount:       attachmentCount,
		AttachmentBytes:       attachmentBytes,
	}
	return sample, nil
}

func (c *Collector) normalizeThroughput(window int64) int64 {
	if window <= 0 {
		return 0
	}
	intervalSeconds := c.cfg.Interval.Seconds()
	if intervalSeconds == 0 {
		return window
	}
	rate := float64(window) * 60 / intervalSeconds
	return int64(math.Round(rate))
}

func (c *Collector) countOnlineUsers(nowMs int64) int64 {
	cutoff := nowMs - c.cfg.OnlineTTL.Milliseconds()
	var total int64
	c.userStates.Range(func(key, value any) bool {
		presence, ok := value.(*userPresence)
		if !ok {
			return true
		}
		if presence.connections.Load() <= 0 {
			c.userStates.Delete(key)
			return true
		}
		if presence.lastActive.Load() >= cutoff {
			total++
		}
		return true
	})
	return total
}

// RecordConnectionOpened 在验证通过时调用。
func (c *Collector) RecordConnectionOpened(userID string) {
	if c == nil || userID == "" {
		return
	}
	c.connCount.Add(1)
	presence := c.ensurePresence(userID)
	presence.connections.Add(1)
	presence.lastActive.Store(time.Now().UnixMilli())
}

// RecordConnectionClosed 在连接关闭时调用。
func (c *Collector) RecordConnectionClosed(userID string) {
	if c == nil || userID == "" {
		return
	}
	if c.connCount.Add(-1) < 0 {
		c.connCount.Store(0)
	}
	if value, ok := c.userStates.Load(userID); ok {
		presence, _ := value.(*userPresence)
		if presence != nil {
			if presence.connections.Add(-1) <= 0 {
				c.userStates.Delete(userID)
			} else {
				presence.lastActive.Store(time.Now().UnixMilli())
			}
		}
	}
}

// RecordUserHeartbeat 在接收到 ping 或业务心跳时调用。
func (c *Collector) RecordUserHeartbeat(userID string) {
	if c == nil || userID == "" {
		return
	}
	presence := c.ensurePresence(userID)
	presence.lastActive.Store(time.Now().UnixMilli())
}

// RecordMessage 在消息写入成功时调用。
func (c *Collector) RecordMessage() {
	if c == nil {
		return
	}
	c.messageWindow.Add(1)
}

// LatestSample 返回最近一次采样结果。
func (c *Collector) LatestSample() (*model.ServiceMetricSample, bool) {
	if c == nil {
		return nil, false
	}
	ptr := c.latestSample.Load()
	if ptr == nil || ptr.TimestampMs == 0 {
		return nil, false
	}
	copy := *ptr
	return &copy, true
}

func (c *Collector) ensurePresence(userID string) *userPresence {
	value, ok := c.userStates.Load(userID)
	if ok {
		if presence, valid := value.(*userPresence); valid {
			return presence
		}
	}
	presence := &userPresence{}
	actual, _ := c.userStates.LoadOrStore(userID, presence)
	if result, ok := actual.(*userPresence); ok {
		return result
	}
	return presence
}

// Interval 返回采样周期。
func (c *Collector) Interval() time.Duration {
	if c == nil || c.cfg.Interval <= 0 {
		return time.Minute
	}
	return c.cfg.Interval
}

// Retention 返回保留时长。
func (c *Collector) Retention() time.Duration {
	if c == nil || c.cfg.Retention <= 0 {
		return 7 * 24 * time.Hour
	}
	return c.cfg.Retention
}

// OnlineTTL 返回在线判定窗口。
func (c *Collector) OnlineTTL() time.Duration {
	if c == nil || c.cfg.OnlineTTL <= 0 {
		return 2 * time.Minute
	}
	return c.cfg.OnlineTTL
}

const attachmentDiskStatsTTL = 24 * time.Hour

var attachmentDiskStatsCache struct {
	mu        sync.Mutex
	updatedAt time.Time
	count     int64
	bytes     int64
}

func getAttachmentDiskStats() (int64, int64) {
	uploadDir := resolveAttachmentUploadDir()
	if strings.TrimSpace(uploadDir) == "" {
		return 0, 0
	}
	now := time.Now()
	attachmentDiskStatsCache.mu.Lock()
	if !attachmentDiskStatsCache.updatedAt.IsZero() && now.Sub(attachmentDiskStatsCache.updatedAt) < attachmentDiskStatsTTL {
		count := attachmentDiskStatsCache.count
		bytes := attachmentDiskStatsCache.bytes
		attachmentDiskStatsCache.mu.Unlock()
		return count, bytes
	}
	attachmentDiskStatsCache.mu.Unlock()

	count, bytes, err := scanDirStats(uploadDir)
	if err != nil {
		log.Printf("metrics: scan attachments failed: %v", err)
		attachmentDiskStatsCache.mu.Lock()
		cachedCount := attachmentDiskStatsCache.count
		cachedBytes := attachmentDiskStatsCache.bytes
		attachmentDiskStatsCache.mu.Unlock()
		return cachedCount, cachedBytes
	}

	attachmentDiskStatsCache.mu.Lock()
	attachmentDiskStatsCache.updatedAt = now
	attachmentDiskStatsCache.count = count
	attachmentDiskStatsCache.bytes = bytes
	attachmentDiskStatsCache.mu.Unlock()
	return count, bytes
}

func resolveAttachmentUploadDir() string {
	cfg := utils.GetConfig()
	if cfg != nil {
		if dir := strings.TrimSpace(cfg.Storage.Local.UploadDir); dir != "" {
			return dir
		}
	}
	return "./data/upload"
}

func scanDirStats(root string) (int64, int64, error) {
	info, err := os.Stat(root)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, nil
	}
	var count int64
	var bytes int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.Type().IsRegular() {
			fileInfo, err := entry.Info()
			if err != nil {
				return nil
			}
			count += 1
			bytes += fileInfo.Size()
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return count, bytes, nil
}
