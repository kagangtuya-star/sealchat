package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"sealchat/service"
)

// writeCounter 用于跟踪写入进度的结构体
type writeCounter struct {
	total   int64
	onWrite func(int64)
}

// Write 实现io.Writer接口
func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	if wc.onWrite != nil {
		wc.onWrite(int64(n))
	}
	return n, nil
}

func downloadLatestRelease() error {
	// 获取最新release信息
	resp, err := http.Get("https://api.github.com/repos/sealdice/sealchat/releases/latest")
	if err != nil {
		return fmt.Errorf("获取release信息失败: %v", err)
	}
	defer resp.Body.Close()

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("解析release信息失败: %v", err)
	}

	// 获取当前系统和架构信息
	osType := runtime.GOOS
	arch := runtime.GOARCH

	// 查找匹配的资源文件
	var targetAsset struct {
		Name string
		URL  string
	}

	for _, asset := range release.Assets {
		// 只检查系统和架构后缀
		suffix := fmt.Sprintf("_%s_%s", osType, arch)
		if strings.Contains(asset.Name, suffix) {
			targetAsset.Name = asset.Name
			targetAsset.URL = asset.BrowserDownloadURL
			break
		}
	}

	if targetAsset.URL == "" {
		return fmt.Errorf("未找到适合当前系统(%s_%s)的安装包", osType, arch)
	}

	fmt.Printf("正在下载最新版本的压缩包: %s\n", targetAsset.Name)
	fmt.Println("请注意，即使当前版本就是最新版本，也会一样下载")
	fmt.Println("根据网络情况，下载可能需要较长时间，请耐心等待")

	// 获取文件大小
	resp, err = http.Head(targetAsset.URL)
	if err != nil {
		return fmt.Errorf("获取文件大小失败: %v", err)
	}
	fileSize := resp.ContentLength
	fmt.Printf("文件大小: %.2f MB\n", float64(fileSize)/(1024*1024))

	resp, err = http.Get(targetAsset.URL)
	if err != nil {
		return fmt.Errorf("下载文件失败: %v", err)
	}
	defer resp.Body.Close()

	partPath := targetAsset.Name + ".part"
	out, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer out.Close()

	// 创建一个计数器来跟踪已下载的字节数
	var downloaded int64 = 0
	lastPrintTime := time.Now()

	// 创建一个代理reader来统计下载进度
	reader := io.TeeReader(resp.Body, &writeCounter{
		total: fileSize,
		onWrite: func(n int64) {
			downloaded += n
			if time.Since(lastPrintTime) >= 10*time.Second {
				progress := float64(downloaded) / float64(fileSize) * 100
				fmt.Printf("已下载: %.1f%% (%.1f MB)\n",
					progress,
					float64(downloaded)/(1024*1024))
				lastPrintTime = time.Now()
			}
		},
	})

	_, err = io.Copy(out, reader)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(partPath)
		return fmt.Errorf("保存文件失败: %v", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("关闭下载文件失败: %v", err)
	}
	_ = os.Remove(targetAsset.Name)
	if err := os.Rename(partPath, targetAsset.Name); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("提交下载文件失败: %v", err)
	}
	if err := service.CacheLegacyDownloadedUpdate(targetAsset.Name); err != nil {
		return fmt.Errorf("归档下载文件失败: %v", err)
	}
	fmt.Printf("文件 %s 下载并归档完成\n", targetAsset.Name)

	return nil
}
