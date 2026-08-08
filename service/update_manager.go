package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

const (
	updateChannelStable = "stable"
	updateChannelTest   = "test"
	updateMaxAssetSize  = int64(256 * 1024 * 1024)
	updateCacheDirName  = "cache"
	updateStageDirName  = "stage"
)

var (
	ErrUpdateBusy        = errors.New("update is already running")
	ErrReleaseChanged    = errors.New("release changed since confirmation")
	ErrUpdateUnsupported = errors.New("self update is unsupported in this environment")
	ErrAlreadyCurrent    = errors.New("selected version is already installed")

	testVersionPattern = regexp.MustCompile(`^sealchat_test_([0-9]{8}-[0-9a-fA-F]{7,40})_`)
	repoPattern        = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

	updateCatalogMu sync.RWMutex
	updateCatalog   *UpdateOverview
	updateRunMu     sync.Mutex
	updateRunActive bool
	updateShutdown  func()
	updateCacheMu   sync.Mutex
)

type updateArtifactKind string

const (
	updateArtifactUpdater updateArtifactKind = "updater"
	updateArtifactStable  updateArtifactKind = updateChannelStable
	updateArtifactTest    updateArtifactKind = updateChannelTest
)

type updateCacheMetadata struct {
	Kind           updateArtifactKind `json:"kind"`
	Channel        string             `json:"channel,omitempty"`
	Platform       string             `json:"platform"`
	ReleaseID      int64              `json:"releaseId"`
	AssetID        int64              `json:"assetId"`
	Version        string             `json:"version,omitempty"`
	Tag            string             `json:"tag,omitempty"`
	PublishedAt    int64              `json:"publishedAt"`
	AssetCreatedAt int64              `json:"assetCreatedAt"`
	AssetName      string             `json:"assetName"`
	Size           int64              `json:"size"`
	Digest         string             `json:"digest,omitempty"`
	DownloadedAt   int64              `json:"downloadedAt"`
}

type updateCacheEntry struct {
	AssetPath string
	MetaPath  string
	Metadata  updateCacheMetadata
	ModTime   time.Time
}

type UpdateAssetInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedAt   int64  `json:"createdAt"`
	DownloadURL string `json:"-"`
	Digest      string `json:"digest"`
}

type UpdateReleaseInfo struct {
	Channel       string           `json:"channel"`
	ReleaseID     int64            `json:"releaseId"`
	Tag           string           `json:"tag"`
	Version       string           `json:"version"`
	Name          string           `json:"name"`
	Body          string           `json:"body"`
	PublishedAt   int64            `json:"publishedAt"`
	HTMLURL       string           `json:"htmlUrl"`
	Prerelease    bool             `json:"prerelease"`
	Asset         *UpdateAssetInfo `json:"asset,omitempty"`
	IsCurrent     bool             `json:"isCurrent"`
	PlatformLabel string           `json:"platformLabel"`
}

type UpdateJobInfo struct {
	Status          string `json:"status"`
	Channel         string `json:"channel"`
	TargetVersion   string `json:"targetVersion"`
	ReleaseID       int64  `json:"releaseId"`
	AssetID         int64  `json:"assetId"`
	AssetName       string `json:"assetName"`
	Progress        int    `json:"progress"`
	Message         string `json:"message"`
	Error           string `json:"error"`
	StartedAt       int64  `json:"startedAt"`
	FinishedAt      int64  `json:"finishedAt"`
	PreviousVersion string `json:"previousVersion"`
}

type UpdateOverview struct {
	CurrentVersion    string             `json:"currentVersion"`
	Supported         bool               `json:"supported"`
	UnsupportedReason string             `json:"unsupportedReason"`
	Platform          string             `json:"platform"`
	LastCheckedAt     int64              `json:"lastCheckedAt"`
	Stable            *UpdateReleaseInfo `json:"stable,omitempty"`
	Test              *UpdateReleaseInfo `json:"test,omitempty"`
	Job               *UpdateJobInfo     `json:"job,omitempty"`
}

func SetUpdateShutdownFunc(fn func()) {
	updateRunMu.Lock()
	updateShutdown = fn
	updateRunMu.Unlock()
}

func GetUpdateOverview(cfg utils.UpdateCheckConfig, force bool) (*UpdateOverview, error) {
	if force {
		CleanupDownloadedUpdates()
	}
	updateCatalogMu.RLock()
	cached := updateCatalog
	updateCatalogMu.RUnlock()
	if force {
		fresh, err := refreshUpdateCatalog(cfg)
		if err != nil {
			return nil, err
		}
		updateCatalogMu.Lock()
		updateCatalog = fresh
		updateCatalogMu.Unlock()
		cached = fresh
	}
	if cached == nil {
		cached = newUpdateOverview()
	}
	ret := *cached
	if !cfg.Enabled {
		ret.Supported = false
		ret.UnsupportedReason = "版本检测与自动更新已在配置中关闭"
	}
	ret.Job = loadUpdateJobInfo()
	return &ret, nil
}

func resolveUpdateExecutablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil && resolved != "" {
		exePath = resolved
	}
	return filepath.Abs(exePath)
}

func updatePlatformKey() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func updateStorageRoot(exeDir string) string {
	return filepath.Join(exeDir, ".sealchat-update")
}

func updateCacheRoot(exeDir string) string {
	return filepath.Join(updateStorageRoot(exeDir), updateCacheDirName)
}

func updateStageRoot(exeDir string) string {
	return filepath.Join(updateStorageRoot(exeDir), updateStageDirName)
}

func updateJobProtectsStage(status string) bool {
	switch status {
	case "preparing", "downloading_updater", "downloading_package", "verifying", "preparing_restart", "restarting":
		return true
	default:
		return false
	}
}

func updateRunIsActive() bool {
	updateRunMu.Lock()
	active := updateRunActive
	updateRunMu.Unlock()
	return active
}

func cacheArtifactDir(exeDir string, kind updateArtifactKind) string {
	return filepath.Join(updateCacheRoot(exeDir), string(kind), updatePlatformKey())
}

func sanitizeCachePart(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "" || value == "." || value == string(filepath.Separator) {
		return "asset"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "asset"
	}
	return builder.String()
}

func cacheArtifactFileName(metadata updateCacheMetadata) string {
	identity := fmt.Sprintf("%d-%d", metadata.ReleaseID, metadata.AssetID)
	if metadata.ReleaseID <= 0 || metadata.AssetID <= 0 {
		identity = fmt.Sprintf("legacy-%d", metadata.DownloadedAt)
	}
	return identity + "-" + sanitizeCachePart(metadata.AssetName)
}

func updateCacheMetadataPath(assetPath string) string {
	return assetPath + ".json"
}

func writeUpdateCacheMetadata(path string, metadata updateCacheMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".part"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func readUpdateCacheMetadata(path string) (updateCacheMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCacheMetadata{}, err
	}
	var metadata updateCacheMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return updateCacheMetadata{}, err
	}
	if metadata.Kind == "" || metadata.AssetName == "" || metadata.Platform == "" {
		return updateCacheMetadata{}, errors.New("invalid update cache metadata")
	}
	return metadata, nil
}

func cacheMetadataMatches(metadata updateCacheMetadata, kind updateArtifactKind, asset *UpdateAssetInfo) bool {
	if asset == nil || metadata.Kind != kind || metadata.Platform != updatePlatformKey() || metadata.AssetID != asset.ID || metadata.AssetName != asset.Name || metadata.Size != asset.Size {
		return false
	}
	want := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	return want == "" || metadata.Digest == want
}

func ensureCachedUpdateAsset(cfg utils.UpdateCheckConfig, kind updateArtifactKind, channel string, release *githubRelease, releaseInfo *UpdateReleaseInfo, asset *UpdateAssetInfo) (string, error) {
	if release == nil && releaseInfo == nil {
		return "", errors.New("update release metadata is empty")
	}
	if asset == nil {
		return "", errors.New("release asset is empty")
	}
	exePath, err := resolveUpdateExecutablePath()
	if err != nil {
		return "", err
	}
	metadata := updateCacheMetadata{
		Kind:         kind,
		Channel:      channel,
		Platform:     updatePlatformKey(),
		ReleaseID:    asset.ID,
		AssetID:      asset.ID,
		AssetName:    asset.Name,
		Size:         asset.Size,
		Digest:       strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:"),
		DownloadedAt: time.Now().UnixMilli(),
	}
	if releaseInfo != nil {
		metadata.ReleaseID = releaseInfo.ReleaseID
		metadata.Version = releaseInfo.Version
		metadata.Tag = releaseInfo.Tag
		metadata.PublishedAt = releaseInfo.PublishedAt
		metadata.AssetCreatedAt = asset.CreatedAt
	} else if release != nil {
		metadata.ReleaseID = release.ID
		metadata.Tag = release.TagName
		metadata.PublishedAt = parseGitHubTime(release.PublishedAt)
		metadata.AssetCreatedAt = asset.CreatedAt
	}
	if metadata.ReleaseID <= 0 {
		metadata.ReleaseID = asset.ID
	}

	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	cacheDir := cacheArtifactDir(filepath.Dir(exePath), kind)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", err
	}
	assetPath := filepath.Join(cacheDir, cacheArtifactFileName(metadata))
	metaPath := updateCacheMetadataPath(assetPath)
	if existing, readErr := readUpdateCacheMetadata(metaPath); readErr == nil && cacheMetadataMatches(existing, kind, asset) {
		if info, statErr := os.Stat(assetPath); statErr == nil && info.Mode().IsRegular() && info.Size() == asset.Size {
			return assetPath, nil
		}
	}

	partPath := assetPath + ".part"
	_ = os.Remove(partPath)
	if err := downloadReleaseAsset(cfg.DownloadProxy, asset, partPath); err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	_ = os.Remove(assetPath)
	if err := os.Rename(partPath, assetPath); err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	_ = os.Remove(metaPath)
	if err := writeUpdateCacheMetadata(metaPath, metadata); err != nil {
		_ = os.Remove(assetPath)
		return "", err
	}
	return assetPath, nil
}

func classifyDownloadedArtifact(name string) (updateArtifactKind, bool) {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".json") {
		return "", false
	}
	if strings.HasPrefix(name, "sealupd-") {
		return updateArtifactUpdater, true
	}
	if strings.HasPrefix(name, "sealchat_test_") {
		return updateArtifactTest, true
	}
	if strings.HasPrefix(name, "sealchat_") {
		return updateArtifactStable, true
	}
	return "", false
}

func updateCacheEntryNewer(left, right updateCacheEntry) bool {
	leftPublished := left.Metadata.PublishedAt
	rightPublished := right.Metadata.PublishedAt
	if leftPublished != rightPublished {
		return leftPublished > rightPublished
	}
	leftCreated := left.Metadata.AssetCreatedAt
	rightCreated := right.Metadata.AssetCreatedAt
	if leftCreated != rightCreated {
		return leftCreated > rightCreated
	}
	if left.Metadata.DownloadedAt != right.Metadata.DownloadedAt {
		return left.Metadata.DownloadedAt > right.Metadata.DownloadedAt
	}
	if !left.ModTime.Equal(right.ModTime) {
		return left.ModTime.After(right.ModTime)
	}
	return left.AssetPath > right.AssetPath
}

func cleanupUpdateCachePlatform(cacheDir string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("update-cleanup: 读取缓存目录失败: %s: %v", cacheDir, err)
		}
		return
	}

	cacheEntries := make([]updateCacheEntry, 0, len(entries))
	knownMeta := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		assetPath := filepath.Join(cacheDir, name)
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".part") {
			_ = os.Remove(assetPath)
			continue
		}
		if strings.HasSuffix(name, ".json") {
			knownMeta[assetPath] = struct{}{}
			continue
		}
		metadataPath := updateCacheMetadataPath(assetPath)
		metadata, metadataErr := readUpdateCacheMetadata(metadataPath)
		info, statErr := entry.Info()
		if metadataErr != nil || statErr != nil || !info.Mode().IsRegular() || info.Size() != metadata.Size {
			_ = os.Remove(assetPath)
			_ = os.Remove(metadataPath)
			continue
		}
		cacheEntries = append(cacheEntries, updateCacheEntry{
			AssetPath: assetPath,
			MetaPath:  metadataPath,
			Metadata:  metadata,
			ModTime:   info.ModTime(),
		})
	}

	sort.Slice(cacheEntries, func(i, j int) bool {
		return updateCacheEntryNewer(cacheEntries[i], cacheEntries[j])
	})
	for index, entry := range cacheEntries {
		if index == 0 {
			continue
		}
		_ = os.Remove(entry.AssetPath)
		_ = os.Remove(entry.MetaPath)
	}
	for metadataPath := range knownMeta {
		assetPath := strings.TrimSuffix(metadataPath, ".json")
		if _, err := os.Stat(assetPath); errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(metadataPath)
		}
	}
}

func migrateLegacyDownloadedArtifact(exeDir, sourcePath string, kind updateArtifactKind, timestamp int64) {
	info, err := os.Stat(sourcePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return
	}
	if timestamp <= 0 {
		timestamp = info.ModTime().UnixMilli()
	}
	metadata := updateCacheMetadata{
		Kind:         kind,
		Channel:      string(kind),
		Platform:     updatePlatformKey(),
		AssetName:    filepath.Base(sourcePath),
		Size:         info.Size(),
		DownloadedAt: timestamp,
	}
	cacheDir := cacheArtifactDir(exeDir, kind)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		log.Printf("update-cleanup: 创建兼容缓存目录失败: %v", err)
		return
	}
	assetPath := filepath.Join(cacheDir, cacheArtifactFileName(metadata))
	if _, err := os.Stat(assetPath); err == nil {
		_ = os.Remove(sourcePath)
		return
	}
	if err := os.Rename(sourcePath, assetPath); err != nil {
		log.Printf("update-cleanup: 迁移旧更新文件失败: %s: %v", sourcePath, err)
		return
	}
	if err := writeUpdateCacheMetadata(updateCacheMetadataPath(assetPath), metadata); err != nil {
		_ = os.Remove(assetPath)
		log.Printf("update-cleanup: 写入旧更新文件元数据失败: %s: %v", assetPath, err)
	}
}

func migrateLegacyUpdateFiles(exeDir, protectedStage string) {
	storageRoot := updateStorageRoot(exeDir)
	entries, err := os.ReadDir(storageRoot)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == updateCacheDirName || entry.Name() == updateStageDirName || entry.Name() == protectedStage {
				continue
			}
			timestamp, parseErr := strconv.ParseInt(entry.Name(), 10, 64)
			if parseErr != nil || timestamp <= 0 {
				continue
			}
			legacyDir := filepath.Join(storageRoot, entry.Name())
			files, readErr := os.ReadDir(legacyDir)
			if readErr != nil {
				continue
			}
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				kind, ok := classifyDownloadedArtifact(file.Name())
				if ok {
					migrateLegacyDownloadedArtifact(exeDir, filepath.Join(legacyDir, file.Name()), kind, timestamp)
				}
			}
		}
	}

	rootEntries, rootErr := os.ReadDir(exeDir)
	if rootErr != nil {
		return
	}
	for _, entry := range rootEntries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".part") && (strings.HasPrefix(entry.Name(), "sealchat_") || strings.HasPrefix(entry.Name(), "sealupd-")) {
			_ = os.Remove(filepath.Join(exeDir, entry.Name()))
			continue
		}
		kind, ok := classifyDownloadedArtifact(entry.Name())
		if ok {
			migrateLegacyDownloadedArtifact(exeDir, filepath.Join(exeDir, entry.Name()), kind, 0)
		}
	}
}

func cleanupUpdateStageTree(exeDir, protectedStage string) {
	storageRoot := updateStorageRoot(exeDir)
	for _, root := range []string{updateStageRoot(exeDir), storageRoot} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if root == storageRoot && (entry.Name() == updateCacheDirName || entry.Name() == updateStageDirName || entry.Name() == protectedStage) {
				continue
			}
			if root == updateStageRoot(exeDir) && entry.Name() == protectedStage {
				continue
			}
			if root == storageRoot {
				if _, err := strconv.ParseInt(entry.Name(), 10, 64); err != nil {
					continue
				}
			}
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func CleanupDownloadedUpdates() {
	if updateRunIsActive() {
		return
	}
	exePath, err := resolveUpdateExecutablePath()
	if err != nil {
		log.Printf("update-cleanup: 获取程序路径失败: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	var job *model.UpdateJobState
	if model.GetDB() != nil {
		job, _ = model.UpdateJobStateGet()
	}
	protectedStage := ""
	if job != nil && updateJobProtectsStage(job.Status) {
		protectedStage = strconv.FormatInt(job.StartedAt, 10)
	}

	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()
	migrateLegacyUpdateFiles(exeDir, protectedStage)
	cacheRoot := updateCacheRoot(exeDir)
	for _, kind := range []updateArtifactKind{updateArtifactUpdater, updateArtifactStable, updateArtifactTest} {
		kindRoot := filepath.Join(cacheRoot, string(kind))
		platformDirs, readErr := os.ReadDir(kindRoot)
		if readErr != nil {
			continue
		}
		for _, platformDir := range platformDirs {
			if platformDir.IsDir() {
				cleanupUpdateCachePlatform(filepath.Join(kindRoot, platformDir.Name()))
			}
		}
	}
	cleanupUpdateStageTree(exeDir, protectedStage)
}

func CacheLegacyDownloadedUpdate(filePath string) error {
	filePath, err := filepath.Abs(strings.TrimSpace(filePath))
	if err != nil {
		return err
	}
	kind, ok := classifyDownloadedArtifact(filepath.Base(filePath))
	if !ok {
		return nil
	}
	exePath, err := resolveUpdateExecutablePath()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()
	migrateLegacyDownloadedArtifact(exeDir, filePath, kind, 0)
	return nil
}

func refreshUpdateCatalog(cfg utils.UpdateCheckConfig) (*UpdateOverview, error) {
	overview := newUpdateOverview()
	overview.LastCheckedAt = time.Now().UnixMilli()
	platform, _ := sealchatPlatformSuffix()
	current := overview.CurrentVersion

	stable, stableErr := fetchChannelRelease(cfg, updateChannelStable, platform)
	test, testErr := fetchChannelRelease(cfg, updateChannelTest, platform)
	if stable != nil {
		stable.IsCurrent = stable.Version == current
		overview.Stable = stable
	}
	if test != nil {
		test.IsCurrent = test.Version == current
		overview.Test = test
	}
	if stableErr != nil && testErr != nil {
		return nil, fmt.Errorf("正式通道: %v; 测试通道: %v", stableErr, testErr)
	}
	return overview, nil
}

func newUpdateOverview() *UpdateOverview {
	current := strings.TrimSpace(utils.BuildVersion)
	_, supported := sealchatPlatformSuffix()
	overview := &UpdateOverview{
		CurrentVersion: current,
		Supported:      supported && current != "",
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
		LastCheckedAt:  0,
	}
	if !supported {
		overview.UnsupportedReason = "当前平台没有可用的 SealChat Release 资产"
	} else if current == "" {
		overview.UnsupportedReason = "当前程序未写入构建版本，不能安全执行自动更新"
	}

	return overview
}

func fetchChannelRelease(cfg utils.UpdateCheckConfig, channel, platformSuffix string) (*UpdateReleaseInfo, error) {
	repo := strings.TrimSpace(cfg.GithubRepo)
	if !repoPattern.MatchString(repo) {
		return nil, fmt.Errorf("invalid github repo: %s", repo)
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if channel == updateChannelTest {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/dev-prerelease", repo)
	}
	release, err := fetchGitHubRelease(endpoint, cfg.GithubToken)
	if err != nil {
		return nil, err
	}
	asset := selectSealchatAsset(release.Assets, channel, platformSuffix)
	if asset == nil {
		return nil, fmt.Errorf("release has no asset for %s", platformSuffix)
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if channel == updateChannelTest {
		matches := testVersionPattern.FindStringSubmatch(asset.Name)
		if len(matches) != 2 {
			return nil, fmt.Errorf("cannot parse test version from asset %s", asset.Name)
		}
		version = matches[1]
	}
	publishedAt := parseGitHubTime(release.PublishedAt)
	if channel == updateChannelTest || publishedAt == 0 {
		publishedAt = parseGitHubTime(asset.CreatedAt)
	}
	return &UpdateReleaseInfo{
		Channel:       channel,
		ReleaseID:     release.ID,
		Tag:           release.TagName,
		Version:       version,
		Name:          release.Name,
		Body:          release.Body,
		PublishedAt:   publishedAt,
		HTMLURL:       release.HTMLURL,
		Prerelease:    release.Prerelease,
		PlatformLabel: runtime.GOOS + "/" + runtime.GOARCH,
		Asset: &UpdateAssetInfo{
			ID:          asset.ID,
			Name:        asset.Name,
			Size:        asset.Size,
			CreatedAt:   parseGitHubTime(asset.CreatedAt),
			DownloadURL: asset.BrowserDownloadURL,
			Digest:      asset.Digest,
		},
	}, nil
}

func fetchGitHubRelease(endpoint, token string) (*githubRelease, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "SealChat-Updater")
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api status=%d", resp.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func selectSealchatAsset(assets []githubReleaseAsset, channel, platformSuffix string) *githubReleaseAsset {
	for i := range assets {
		name := assets[i].Name
		if !strings.HasSuffix(name, platformSuffix) {
			continue
		}
		if channel == updateChannelTest && strings.HasPrefix(name, "sealchat_test_") {
			return &assets[i]
		}
		if channel == updateChannelStable && strings.HasPrefix(name, "sealchat_") && !strings.HasPrefix(name, "sealchat_test_") {
			return &assets[i]
		}
	}
	return nil
}

func sealchatPlatformSuffix() (string, bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "_linux_amd64.tar.gz", true
	case "linux/arm64":
		return "_linux_arm64.tar.gz", true
	case "windows/amd64":
		return "_windows_amd64.zip", true
	default:
		return "", false
	}
}

func sealupdAssetName() (string, bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "sealupd-linux-x86_64.tar.gz", true
	case "linux/arm64":
		return "sealupd-linux-aarch64.tar.gz", true
	case "windows/amd64":
		return "sealupd-windows-x86_64.zip", true
	default:
		return "", false
	}
}

func parseGitHubTime(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func loadUpdateJobInfo() *UpdateJobInfo {
	state, err := model.UpdateJobStateGet()
	if err != nil || state == nil {
		return nil
	}
	return jobInfoFromModel(state)
}

func jobInfoFromModel(state *model.UpdateJobState) *UpdateJobInfo {
	if state == nil {
		return nil
	}
	return &UpdateJobInfo{
		Status: state.Status, Channel: state.Channel, TargetVersion: state.TargetVersion,
		ReleaseID: state.ReleaseID, AssetID: state.AssetID, AssetName: state.AssetName,
		Progress: state.Progress, Message: state.Message, Error: state.Error,
		StartedAt: state.StartedAt, FinishedAt: state.FinishedAt, PreviousVersion: state.PreviousVersion,
	}
}

func StartUpdate(cfg utils.UpdateCheckConfig, channel string, expectedReleaseID, expectedAssetID int64) (*UpdateJobInfo, error) {
	if !cfg.Enabled {
		return nil, ErrUpdateUnsupported
	}
	if channel != updateChannelStable && channel != updateChannelTest {
		return nil, fmt.Errorf("invalid update channel")
	}
	current := strings.TrimSpace(utils.BuildVersion)
	platform, supported := sealchatPlatformSuffix()
	if !supported || current == "" {
		return nil, ErrUpdateUnsupported
	}

	updateRunMu.Lock()
	if updateRunActive {
		updateRunMu.Unlock()
		return nil, ErrUpdateBusy
	}
	updateRunActive = true
	updateRunMu.Unlock()

	release, err := fetchChannelRelease(cfg, channel, platform)
	if err != nil {
		finishUpdateRun()
		return nil, err
	}
	if release.Asset == nil || release.ReleaseID != expectedReleaseID || release.Asset.ID != expectedAssetID {
		finishUpdateRun()
		return nil, ErrReleaseChanged
	}
	if release.Version == current {
		finishUpdateRun()
		return nil, ErrAlreadyCurrent
	}

	job := &model.UpdateJobState{
		Status: "preparing", Channel: channel, TargetVersion: release.Version,
		ReleaseID: release.ReleaseID, AssetID: release.Asset.ID, AssetName: release.Asset.Name,
		Progress: 5, Message: "正在准备更新", StartedAt: time.Now().UnixMilli(), PreviousVersion: current,
	}
	if err := model.UpdateJobStateUpsert(job); err != nil {
		finishUpdateRun()
		return nil, err
	}
	go runPreparedUpdate(cfg, release, job)
	return jobInfoFromModel(job), nil
}

func finishUpdateRun() {
	updateRunMu.Lock()
	updateRunActive = false
	updateRunMu.Unlock()
}

func saveUpdateJob(job *model.UpdateJobState, status string, progress int, message string, jobErr error) {
	job.Status = status
	job.Progress = progress
	job.Message = message
	if jobErr != nil {
		job.Error = jobErr.Error()
		job.FinishedAt = time.Now().UnixMilli()
	}
	_ = model.UpdateJobStateUpsert(job)
}

func runPreparedUpdate(cfg utils.UpdateCheckConfig, release *UpdateReleaseInfo, job *model.UpdateJobState) {
	fail := func(err error) {
		saveUpdateJob(job, "failed", job.Progress, "更新失败", err)
		finishUpdateRun()
	}

	saveUpdateJob(job, "preparing", 5, "正在执行更新前数据库备份", nil)
	appCfg := utils.GetConfig()
	if appCfg == nil {
		fail(errors.New("更新前数据库备份失败: 配置未加载"))
		return
	}
	if _, backupErr := ExecuteBackup(appCfg); backupErr != nil {
		fail(fmt.Errorf("更新前数据库备份失败: %w", backupErr))
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		fail(err)
		return
	}
	if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil && resolved != "" {
		exePath = resolved
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		fail(err)
		return
	}
	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)
	stageDir := filepath.Join(updateStageRoot(exeDir), strconv.FormatInt(job.StartedAt, 10))
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		fail(fmt.Errorf("创建更新暂存目录失败: %w", err))
		return
	}

	updaterRelease, updaterAsset, err := fetchSealupdAsset(cfg)
	if err != nil {
		fail(err)
		return
	}
	saveUpdateJob(job, "downloading_updater", 15, "正在下载 Sealupd", nil)
	updaterArchive, err := ensureCachedUpdateAsset(cfg, updateArtifactUpdater, "", updaterRelease, nil, updaterAsset)
	if err != nil {
		fail(fmt.Errorf("下载 Sealupd 失败: %w", err))
		return
	}

	updaterExtracted := filepath.Join(stageDir, updaterExecutableName())
	if err := extractArchiveFileCandidates(updaterArchive, updaterArchiveBinaryCandidates(updaterAsset.Name), updaterExtracted); err != nil {
		fail(fmt.Errorf("解压 Sealupd 失败: %w", err))
		return
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(updaterExtracted, 0o755); err != nil {
			fail(fmt.Errorf("设置 Sealupd 权限失败: %w", err))
			return
		}
	}

	saveUpdateJob(job, "downloading_package", 35, "正在下载 SealChat 更新包", nil)
	packageKind := updateArtifactStable
	if release.Channel == updateChannelTest {
		packageKind = updateArtifactTest
	}
	packagePath, err := ensureCachedUpdateAsset(cfg, packageKind, release.Channel, nil, release, release.Asset)
	if err != nil {
		fail(fmt.Errorf("下载 SealChat 更新包失败: %w", err))
		return
	}

	newBinary := filepath.Join(stageDir, "new-sealchat-binary")
	if runtime.GOOS == "windows" {
		newBinary += ".exe"
	}
	saveUpdateJob(job, "verifying", 65, "正在校验并提取主程序", nil)
	archiveBinaryName := "sealchat-server"
	if runtime.GOOS == "windows" {
		archiveBinaryName += ".exe"
	}
	if err := extractArchiveFile(packagePath, archiveBinaryName, newBinary); err != nil {
		fail(fmt.Errorf("提取 SealChat 主程序失败: %w", err))
		return
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(newBinary, 0o755); err != nil {
			fail(fmt.Errorf("设置新主程序权限失败: %w", err))
			return
		}
	}

	updatePackage := filepath.Join(stageDir, "sealchat-binary-update.tar.gz")
	if runtime.GOOS == "windows" {
		updatePackage = filepath.Join(stageDir, "sealchat-binary-update.zip")
	}
	if err := createSingleBinaryPackage(newBinary, exeName, updatePackage); err != nil {
		fail(fmt.Errorf("生成单文件更新包失败: %w", err))
		return
	}

	updaterPath := filepath.Join(exeDir, updaterExecutableName())
	_ = os.Remove(updaterPath)
	if err := os.Rename(updaterExtracted, updaterPath); err != nil {
		fail(fmt.Errorf("放置 Sealupd 失败: %w", err))
		return
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(updaterPath, 0o755); err != nil {
			fail(fmt.Errorf("设置 Sealupd 权限失败: %w", err))
			return
		}
	}
	if err := rotateSealupdBackup(exePath); err != nil {
		fail(fmt.Errorf("轮换旧版本备份失败: %w", err))
		return
	}

	saveUpdateJob(job, "preparing_restart", 90, "即将重启服务", nil)
	cmd := exec.Command(updaterPath,
		"--package", updatePackage,
		"--binary-name", exeName,
		"--pid", strconv.Itoa(os.Getpid()),
	)
	cmd.Dir = exeDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fail(fmt.Errorf("启动 Sealupd 失败: %w", err))
		return
	}
	_ = cmd.Process.Release()
	saveUpdateJob(job, "restarting", 95, "Sealupd 已启动，服务正在重启", nil)

	updateRunMu.Lock()
	shutdown := updateShutdown
	updateRunMu.Unlock()
	if shutdown == nil {
		fail(errors.New("更新关闭回调未初始化"))
		return
	}
	time.Sleep(900 * time.Millisecond)
	shutdown()
}

func rotateSealupdBackup(exePath string) error {
	backupPath := exePath + "_old"
	if runtime.GOOS == "windows" {
		backupPath = exePath + ".old"
	}
	if _, err := os.Stat(backupPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	previousPath := backupPath + ".previous"
	if err := os.Remove(previousPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(backupPath, previousPath)
}

func fetchSealupdAsset(cfg utils.UpdateCheckConfig) (*githubRelease, *UpdateAssetInfo, error) {
	repo := strings.TrimSpace(cfg.SealupdRepo)
	if !repoPattern.MatchString(repo) {
		return nil, nil, fmt.Errorf("invalid sealupd repo: %s", repo)
	}
	name, supported := sealupdAssetName()
	if !supported {
		return nil, nil, ErrUpdateUnsupported
	}
	release, err := fetchGitHubRelease(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), cfg.GithubToken)
	if err != nil {
		return nil, nil, err
	}
	for _, asset := range release.Assets {
		if asset.Name == name {
			return release, &UpdateAssetInfo{
				ID: asset.ID, Name: asset.Name, Size: asset.Size,
				CreatedAt: parseGitHubTime(asset.CreatedAt), DownloadURL: asset.BrowserDownloadURL, Digest: asset.Digest,
			}, nil
		}
	}
	return nil, nil, fmt.Errorf("sealupd release has no asset %s", name)
}

func updaterExecutableName() string {
	if runtime.GOOS == "windows" {
		return "sealupd.exe"
	}
	return "sealupd"
}

func updaterArchiveBinaryCandidates(assetName string) []string {
	assetBase := normalizeArchiveMemberName(assetName)
	if assetBase == "." {
		assetBase = ""
	}
	lowerAssetBase := strings.ToLower(assetBase)
	switch {
	case strings.HasSuffix(lowerAssetBase, ".tar.gz"):
		assetBase = assetBase[:len(assetBase)-len(".tar.gz")]
	case strings.HasSuffix(lowerAssetBase, ".zip"):
		assetBase = assetBase[:len(assetBase)-len(".zip")]
	}

	candidates := make([]string, 0, 2)
	addCandidate := func(name string) {
		name = normalizeArchiveMemberName(name)
		if name == "" {
			return
		}
		for _, existing := range candidates {
			if existing == name {
				return
			}
		}
		candidates = append(candidates, name)
	}

	if assetBase != "" && runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(assetBase), ".exe") {
		assetBase += ".exe"
	}
	if assetBase != "" {
		addCandidate(assetBase)
	}
	addCandidate(updaterExecutableName())
	return candidates
}

func downloadReleaseAsset(proxy string, asset *UpdateAssetInfo, destination string) error {
	if asset == nil || asset.DownloadURL == "" {
		return errors.New("release asset download url is empty")
	}
	if asset.Size <= 0 || asset.Size > updateMaxAssetSize {
		return fmt.Errorf("invalid asset size: %d", asset.Size)
	}
	downloadURL, err := proxiedDownloadURL(proxy, asset.DownloadURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SealChat-Updater")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download status=%d", resp.StatusCode)
	}
	if resp.ContentLength > updateMaxAssetSize {
		return fmt.Errorf("download is too large: %d", resp.ContentLength)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, updateMaxAssetSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	if written > updateMaxAssetSize || written != asset.Size {
		_ = os.Remove(destination)
		return fmt.Errorf("download size mismatch: got %d want %d", written, asset.Size)
	}
	want := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	if len(want) != sha256.Size*2 {
		_ = os.Remove(destination)
		return errors.New("release asset has no valid sha256 digest")
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		_ = os.Remove(destination)
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}
	return nil
}

func proxiedDownloadURL(proxy, source string) (string, error) {
	sourceURL, err := url.Parse(source)
	if err != nil || sourceURL.Scheme != "https" || !strings.EqualFold(sourceURL.Hostname(), "github.com") {
		return "", errors.New("release asset url is not an allowed GitHub URL")
	}
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return source, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.Host == "" {
		return "", errors.New("download proxy is invalid")
	}
	return strings.TrimRight(proxy, "/") + "/" + source, nil
}

func archivePathSafe(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(name)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func extractArchiveFile(archivePath, wantedBase, destination string) error {
	return extractArchiveFileCandidates(archivePath, []string{wantedBase}, destination)
}

func extractArchiveFileCandidates(archivePath string, wantedBases []string, destination string) error {
	candidates := normalizeArchiveMemberNames(wantedBases)
	if len(candidates) == 0 {
		return errors.New("archive member name is empty")
	}
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractZipFileCandidates(archivePath, candidates, destination)
	}
	return extractTarGzFileCandidates(archivePath, candidates, destination)
}

func extractZipFile(archivePath, wantedBase, destination string) error {
	return extractZipFileCandidates(archivePath, []string{wantedBase}, destination)
}

func extractZipFileCandidates(archivePath string, wantedBases []string, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	entries := make([]string, 0, 8)
	for _, entry := range reader.File {
		appendArchiveEntrySummary(&entries, entry.Name)
		if !archivePathSafe(entry.Name) || !entry.Mode().IsRegular() || !archiveMemberMatches(entry.Name, wantedBases) {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeExtractedFile(src, destination)
		_ = src.Close()
		return err
	}
	return archiveMissingMemberError(archivePath, wantedBases, entries)
}

func extractTarGzFile(archivePath, wantedBase, destination string) error {
	return extractTarGzFileCandidates(archivePath, []string{wantedBase}, destination)
}

func extractTarGzFileCandidates(archivePath string, wantedBases []string, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	entries := make([]string, 0, 8)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		appendArchiveEntrySummary(&entries, header.Name)
		if !archivePathSafe(header.Name) || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || !archiveMemberMatches(header.Name, wantedBases) {
			continue
		}
		return writeExtractedFile(io.LimitReader(reader, updateMaxAssetSize+1), destination)
	}
	return archiveMissingMemberError(archivePath, wantedBases, entries)
}

func normalizeArchiveMemberNames(names []string) []string {
	ret := make([]string, 0, len(names))
	for _, name := range names {
		name = normalizeArchiveMemberName(name)
		if name == "" {
			continue
		}
		duplicate := false
		for _, existing := range ret {
			if existing == name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			ret = append(ret, name)
		}
	}
	return ret
}

func normalizeArchiveMemberName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return ""
	}
	return path.Base(name)
}

func archiveMemberMatches(name string, wantedBases []string) bool {
	base := normalizeArchiveMemberName(name)
	for _, wanted := range wantedBases {
		if base == wanted {
			return true
		}
	}
	return false
}

func appendArchiveEntrySummary(entries *[]string, name string) {
	const maxEntries = 8
	if entries == nil || len(*entries) >= maxEntries {
		return
	}
	name = strings.TrimSpace(name)
	if len(name) > 120 {
		name = name[:120] + "..."
	}
	*entries = append(*entries, name)
}

func archiveMissingMemberError(archivePath string, wantedBases, entries []string) error {
	archiveName := filepath.Base(archivePath)
	if archiveName == "" || archiveName == "." {
		archiveName = archivePath
	}
	wanted := strings.Join(wantedBases, ", ")
	if len(entries) == 0 {
		return fmt.Errorf("archive %s does not contain any of [%s] (entries: none)", archiveName, wanted)
	}
	entrySummary := strings.Join(entries, ", ")
	if len(entries) >= 8 {
		entrySummary += ", ..."
	}
	return fmt.Errorf("archive %s does not contain any of [%s] (entries: %s)", archiveName, wanted, entrySummary)
}

func writeExtractedFile(src io.Reader, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(src, updateMaxAssetSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written <= 0 || written > updateMaxAssetSize {
		return fmt.Errorf("invalid extracted binary size: %d", written)
	}
	return nil
}

func createSingleBinaryPackage(binaryPath, archiveName, destination string) error {
	if runtime.GOOS == "windows" {
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		writer := zip.NewWriter(file)
		entry, err := writer.Create(archiveName)
		if err == nil {
			err = copyFileIntoWriter(binaryPath, entry)
		}
		closeZipErr := writer.Close()
		closeFileErr := file.Close()
		if err != nil {
			return err
		}
		if closeZipErr != nil {
			return closeZipErr
		}
		return closeFileErr
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gz)
	err = tarWriter.WriteHeader(&tar.Header{Name: archiveName, Mode: 0o755, Size: info.Size(), ModTime: time.Now()})
	if err == nil {
		err = copyFileIntoWriter(binaryPath, tarWriter)
	}
	closeTarErr := tarWriter.Close()
	closeGzErr := gz.Close()
	closeFileErr := file.Close()
	if err != nil {
		return err
	}
	if closeTarErr != nil {
		return closeTarErr
	}
	if closeGzErr != nil {
		return closeGzErr
	}
	return closeFileErr
}

func copyFileIntoWriter(source string, destination io.Writer) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}

func ReconcileUpdateJob(currentVersion string) {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" {
		return
	}
	job, err := model.UpdateJobStateGet()
	if err != nil || job == nil || (job.Status != "restarting" && job.Status != "preparing_restart") {
		return
	}
	job.FinishedAt = time.Now().UnixMilli()
	job.Progress = 100
	if job.TargetVersion == currentVersion {
		job.Status = "succeeded"
		job.Message = "更新完成"
		job.Error = ""
	} else {
		job.Status = "failed"
		job.Message = "更新后版本未生效"
		job.Error = fmt.Sprintf("当前版本 %s，目标版本 %s", currentVersion, job.TargetVersion)
	}
	_ = model.UpdateJobStateUpsert(job)
	if job.Status == "succeeded" {
		go cleanupUpdateStage(job.StartedAt)
	}
}

func cleanupUpdateStage(startedAt int64) {
	if startedAt <= 0 {
		return
	}
	time.Sleep(5 * time.Second)
	exePath, err := resolveUpdateExecutablePath()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exePath)
	_ = os.RemoveAll(filepath.Join(updateStageRoot(exeDir), strconv.FormatInt(startedAt, 10)))
	_ = os.RemoveAll(filepath.Join(updateStorageRoot(exeDir), strconv.FormatInt(startedAt, 10)))
}
