package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

type UpdateCheckWorkerConfig struct {
	IntervalSec   int
	GithubRepo    string
	GithubToken   string
	CurrentVersion string
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

type githubReleaseFetchResult struct {
	Release      githubRelease
	ETag         string
	LastModified string
	NotModified  bool
}

var updateCheckWorkerOnce sync.Once

func StartUpdateCheckWorker(cfg UpdateCheckWorkerConfig) {
	updateCheckWorkerOnce.Do(func() {
		if strings.TrimSpace(cfg.GithubRepo) == "" {
			log.Println("update-check: github repo not configured")
			return
		}
		log.Println("update-check: Worker 启动")
		go runUpdateCheckWorker(cfg)
	})
}

func runUpdateCheckWorker(cfg UpdateCheckWorkerConfig) {
	interval := cfg.IntervalSec
	if interval <= 0 {
		interval = 6 * 60 * 60
	}

	runUpdateCheckOnce(cfg)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		runUpdateCheckOnce(cfg)
	}
}

func UpdateCheckOnce(cfg UpdateCheckWorkerConfig) {
	runUpdateCheckOnce(cfg)
}

func SyncUpdateCurrentVersion(currentVersion string) {
	current := strings.TrimSpace(currentVersion)
	if current == "" {
		return
	}
	state, err := model.UpdateCheckStateGet()
	if err != nil {
		log.Printf("update-check: 读取状态失败: %v", err)
		return
	}
	if state == nil {
		state = &model.UpdateCheckState{}
	}
	state.CurrentVersion = current
	if err := model.UpdateCheckStateUpsert(state); err != nil {
		log.Printf("update-check: 更新状态失败: %v", err)
	}
}

func runUpdateCheckOnce(cfg UpdateCheckWorkerConfig) {
	state, err := model.UpdateCheckStateGet()
	if err != nil {
		log.Printf("update-check: 读取状态失败: %v", err)
		return
	}
	if state == nil {
		state = &model.UpdateCheckState{}
	}
	if current := strings.TrimSpace(cfg.CurrentVersion); current != "" {
		state.CurrentVersion = current
	}

	result, err := fetchLatestRelease(cfg.GithubRepo, cfg.GithubToken, state.ETag, state.LastModified)
	if err != nil {
		log.Printf("update-check: 拉取 release 失败: %v", err)
		return
	}

	now := time.Now().UnixMilli()
	if result.NotModified {
		state.LastCheckedAt = now
		if err := model.UpdateCheckStateUpsert(state); err != nil {
			log.Printf("update-check: 更新状态失败: %v", err)
		}
		return
	}

	publishedAtMs := int64(0)
	if ts := strings.TrimSpace(result.Release.PublishedAt); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			publishedAtMs = parsed.UnixMilli()
		}
	}

	state.LatestTag = strings.TrimSpace(result.Release.TagName)
	state.LatestName = strings.TrimSpace(result.Release.Name)
	state.LatestBody = result.Release.Body
	state.LatestPublishedAt = publishedAtMs
	state.LatestHtmlURL = strings.TrimSpace(result.Release.HTMLURL)
	state.LastCheckedAt = now
	state.ETag = result.ETag
	state.LastModified = result.LastModified

	hasUpdate := state.CurrentVersion != "" && isLatestNewer(state.CurrentVersion, state.LatestTag)
	if hasUpdate && state.LatestTag != "" && state.LatestTag != state.LastNotifiedTag {
		if err := notifyAdminsOfUpdate(state); err != nil {
			log.Printf("update-check: 通知管理员失败: %v", err)
		} else {
			state.LastNotifiedTag = state.LatestTag
		}
	}

	if err := model.UpdateCheckStateUpsert(state); err != nil {
		log.Printf("update-check: 更新状态失败: %v", err)
	}
}

func fetchLatestRelease(repo, token, etag, lastModified string) (*githubReleaseFetchResult, error) {
	if strings.TrimSpace(repo) == "" {
		return nil, fmt.Errorf("github repo is empty")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	if strings.TrimSpace(etag) != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if strings.TrimSpace(lastModified) != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &githubReleaseFetchResult{NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api status=%d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &githubReleaseFetchResult{
		Release:      release,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

func parseVersionParts(value string) (int, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), "-", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	datePart := strings.TrimSpace(parts[0])
	shaPart := strings.TrimSpace(parts[1])
	if datePart == "" || shaPart == "" {
		return 0, "", false
	}
	dateInt, err := strconv.Atoi(datePart)
	if err != nil {
		return 0, "", false
	}
	return dateInt, shaPart, true
}

func isLatestNewer(current, latest string) bool {
	if current == "" || latest == "" {
		return false
	}
	currentDate, currentSha, currentOK := parseVersionParts(current)
	latestDate, latestSha, latestOK := parseVersionParts(latest)
	if currentOK && latestOK {
		if latestDate != currentDate {
			return latestDate > currentDate
		}
		return strings.Compare(latestSha, currentSha) > 0
	}
	if !currentOK && latestOK {
		return true
	}
	return latest != current
}

func IsLatestNewer(current, latest string) bool {
	return isLatestNewer(current, latest)
}

func notifyAdminsOfUpdate(state *model.UpdateCheckState) error {
	if state == nil {
		return nil
	}
	adminIDs, err := model.UserRoleMappingUserIdListByRoleId("sys-admin")
	if err != nil {
		return err
	}
	if len(adminIDs) == 0 {
		return nil
	}
	title := fmt.Sprintf("发现新版本 %s", state.LatestTag)
	items := make([]*model.TimelineModel, 0, len(adminIDs))
	for _, userID := range adminIDs {
		if strings.TrimSpace(userID) == "" {
			continue
		}
		items = append(items, &model.TimelineModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			Type:              "system.update",
			Title:             title,
			Brief:             state.LatestName,
			ReceiverId:        userID,
			RelatedType:       "release",
			RelatedID:         state.LatestTag,
			LocPostType:       "release",
			LocPostID:         state.LatestHtmlURL,
			IsRead:            false,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return model.GetDB().CreateInBatches(items, 50).Error
}
