package downloader

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v50/github"
)

type ReleaseInfo struct {
	Launcher    string               `json:"launcher"`
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	PublishedAt time.Time            `json:"published_at"`
	IsLatest    bool                 `json:"is_latest"`
	Assets      []ReleaseAssetSimple `json:"assets"`
}

type ReleaseAssetSimple struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

type Downloader struct {
	httpClient         *http.Client
	semaphore          chan struct{}
	insecureSkipVerify bool
}

func NewDownloader(timeoutMinutes, concurrentDownloads int, insecureSkipVerify bool) *Downloader {
	if concurrentDownloads <= 0 {
		concurrentDownloads = 3 // 如果无效，默认为 3
	}
	var transport http.RoundTripper = http.DefaultTransport
	if insecureSkipVerify {
		transport = insecureTransport()
	}
	return &Downloader{
		httpClient:         &http.Client{Timeout: time.Duration(timeoutMinutes) * time.Minute, Transport: transport},
		semaphore:          make(chan struct{}, concurrentDownloads),
		insecureSkipVerify: insecureSkipVerify,
	}
}

// insecureTransport 返回跳过 TLS 证书校验的传输层（由 tls_skip_verify 配置显式开启）。
func insecureTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	}
	t.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // 由 tls_skip_verify 配置显式开启
	return t
}

func (d *Downloader) DownloadLatest(ctx context.Context, launcher string, destBase string, proxyURL string, assetProxyURL string, xgetEnabled bool, xgetDomain string, rel *github.RepositoryRelease, serverAddress string, serverPort int, downloadUrlBase string, isLatest bool, assetDigests map[string]string) (string, error) {
	if rel == nil {
		return "", errors.New("release 为空")
	}
	version := rel.GetTagName()
	if version == "" {
		version = rel.GetName()
		if version == "" {
			version = fmt.Sprintf("%d", rel.GetID())
		}
	}
	if !isSafePathComponent(launcher) || !isSafePathComponent(version) {
		return "", fmt.Errorf("launcher 或版本包含非法路径字符")
	}
	dir := filepath.Join(destBase, launcher, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}

	var info ReleaseInfo
	info.Launcher = launcher
	info.TagName = rel.GetTagName()
	info.Name = rel.GetName()
	info.PublishedAt = rel.GetPublishedAt().Time
	info.IsLatest = isLatest
	for _, a := range rel.Assets {
		var downloadURL string
		if downloadUrlBase != "" {
			// 如果提供了 downloadUrlBase，则直接使用它。
			// 确保 downloadUrlBase 具有协议头，如果没有则默认为 http://
			baseURL := downloadUrlBase
			if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
				baseURL = "http://" + baseURL
			}
			baseURL = strings.TrimRight(baseURL, "/")
			downloadURL = fmt.Sprintf("%s/download/%s/%s/%s", baseURL, launcher, version, a.GetName())
		} else if serverAddress != "" {
			downloadURL = FormatDownloadURL(serverAddress, serverPort, "", launcher, version, a.GetName())
		} else {
			publicIP, err := getPublicIP()
			if err != nil {
				log.Printf("无法获取公网 IP: %v。回退到资源 %s 的 GitHub URL", err, a.GetName())
				downloadURL = a.GetBrowserDownloadURL()
			} else {
				downloadURL = FormatDownloadURL("", serverPort, publicIP, launcher, version, a.GetName())
			}
		}
		info.Assets = append(info.Assets, ReleaseAssetSimple{
			Name: a.GetName(),
			URL:  downloadURL,
			Size: a.GetSize(),
		})
	}

	indexPath := filepath.Join(dir, "index.json")

	client := d.httpClient
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return "", fmt.Errorf("解析代理URL失败: %w", err)
		}
		// 为代理创建新的客户端，因为默认客户端可能是共享的
		transport := &http.Transport{
			Proxy:               http.ProxyURL(proxy),
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		}
		if d.insecureSkipVerify {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 由 tls_skip_verify 配置显式开启
		}
		client = &http.Client{
			Timeout:   d.httpClient.Timeout,
			Transport: transport,
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(rel.Assets))
	hashes := make([]string, len(rel.Assets))

	for i, asset := range rel.Assets {
		wg.Add(1)
		go func(i int, asset *github.ReleaseAsset) {
			defer wg.Done()
			d.semaphore <- struct{}{}
			defer func() { <-d.semaphore }()

			hash, err := d.ensureAssetHashed(ctx, client, asset, dir, assetProxyURL, xgetEnabled, xgetDomain, assetDigests[asset.GetName()])
			if err != nil {
				errCh <- err
			}
			hashes[i] = hash
		}(i, asset)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return "", err
		}
	}

	// 元数据与实际落盘文件保持一致：index.json 内写入每个资产的 SHA-256
	for i := range info.Assets {
		info.Assets[i].SHA256 = hashes[i]
	}

	// 所有资产下载完成后再写 index.json 并生效，
	// 避免客户端瞬间拿到引用尚未存在资产的索引。
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 index.json 失败: %w", err)
	}

	// 检查 index.json 是否已存在且内容一致
	writeIndex := true
	if existingContent, err := os.ReadFile(indexPath); err == nil {
		if string(existingContent) == string(b) {
			writeIndex = false
		}
	}

	if writeIndex {
		if err := os.WriteFile(indexPath, b, 0o644); err != nil {
			return "", fmt.Errorf("写入 index.json 失败: %w", err)
		}
		log.Printf("已将版本信息写入 %s", indexPath)
	}

	return indexPath, nil
}

// 缓存公网 IP，避免重复请求
var (
	publicIP   string
	publicIPMu sync.RWMutex
)

func getPublicIP() (string, error) {
	publicIPMu.RLock()
	cachedIP := publicIP
	publicIPMu.RUnlock()

	if cachedIP != "" {
		return cachedIP, nil
	}

	publicIPMu.Lock()
	defer publicIPMu.Unlock()

	if publicIP != "" {
		return publicIP, nil
	}

	resp, err := http.Get("https://ifconfig.me/ip")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ipStr := strings.TrimSpace(string(ipBytes))
	if ipStr == "" {
		return "", errors.New("empty response from ifconfig.me")
	}

	publicIP = ipStr
	return publicIP, nil
}

func isSafePathComponent(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, `/\\`)
}

// ensureAssetHashed 确保资产文件就位并返回其 SHA-256（十六进制）。
// expectHex 非空时（来自 GitHub API 的 digest 字段），本地文件每次扫描都会与之比对，
// 不一致则重新下载校验；两次仍不一致仅告警并返回本地实际哈希，保证元数据与所服务文件一致。
func (d *Downloader) ensureAssetHashed(ctx context.Context, client *http.Client, asset *github.ReleaseAsset, dir, assetProxyURL string, xgetEnabled bool, xgetDomain string, expectHex string) (string, error) {
	name := asset.GetName()
	outfile := filepath.Join(dir, name)

	if stat, err := os.Stat(outfile); err == nil {
		if stat.Size() == int64(asset.GetSize()) {
			hash, herr := sha256FileHex(outfile)
			if herr != nil {
				return "", herr
			}
			if expectHex == "" || hash == expectHex {
				return hash, nil
			}
			log.Printf("[hash] %s 本地文件与 GitHub digest 不一致，重新下载", name)
			if rerr := os.Remove(outfile); rerr != nil && !os.IsNotExist(rerr) {
				return "", rerr
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			// 重试前移除上次下载的可疑文件，避免 downloadAsset 按大小跳过
			if err := os.Remove(outfile); err != nil && !os.IsNotExist(err) {
				return "", err
			}
		}
		if err := d.downloadAsset(ctx, client, asset, dir, assetProxyURL, xgetEnabled, xgetDomain); err != nil {
			return "", err
		}
		if _, err := os.Stat(outfile); err != nil {
			// 无下载链接被跳过等情况：与旧行为一致，不算失败
			return "", nil
		}
		hash, err := sha256FileHex(outfile)
		if err != nil {
			return "", err
		}
		if expectHex == "" || hash == expectHex {
			return hash, nil
		}
		log.Printf("[hash] 资源 %s 第 %d/2 次下载校验未通过 (本地 %s / GitHub %s)", name, attempt, hash, expectHex)
	}
	// 两次都不匹配：返回本地实际哈希（元数据必须与所服务文件一致）
	return sha256FileHex(outfile)
}

// sha256FileHex 流式计算文件 SHA-256，返回十六进制字符串。
func sha256FileHex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (d *Downloader) downloadAsset(ctx context.Context, client *http.Client, asset *github.ReleaseAsset, dir, assetProxyURL string, xgetEnabled bool, xgetDomain string) error {
	name := asset.GetName()
	if !isSafePathComponent(name) {
		return fmt.Errorf("资源文件名包含非法路径字符: %q", name)
	}
	outfile := filepath.Join(dir, name)

	if fileInfo, err := os.Stat(outfile); err == nil {
		if fileInfo.Size() == int64(asset.GetSize()) {
			// 文件大小一致，认为是同一个文件，跳过下载且不打印日志
			return nil
		}
		log.Printf("文件 %s 已存在但大小不一致 (本地: %d, 远程: %d)，将重新下载。", name, fileInfo.Size(), asset.GetSize())
	}

	downloadURL := asset.GetBrowserDownloadURL()
	if downloadURL != "" && assetProxyURL != "" {
		downloadURL = assetProxyURL + downloadURL
	}
	if downloadURL != "" && xgetEnabled && strings.HasPrefix(downloadURL, "https://github.com/") {
		downloadURL = strings.Replace(downloadURL, "https://github.com/", xgetDomain+"/gh/", 1)
	}
	if downloadURL == "" {
		log.Printf("资源 %s 没有下载链接，跳过", name)
		return nil
	}
	if name == "" {
		name = filepath.Base(downloadURL)
	}
	log.Printf("开始下载 %s 到 %s", downloadURL, outfile)

	partial := outfile + ".partial"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			lastErr = fmt.Errorf("状态码: %d", resp.StatusCode)
			resp.Body.Close()
		} else {
			lastErr = err
		}
		if attempt == 2 {
			return fmt.Errorf("下载资源 %s 失败: %w", downloadURL, lastErr)
		}
		log.Printf("下载 %s 失败，5秒后重试...", downloadURL)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer resp.Body.Close()

	f, err := os.Create(partial)
	if err != nil {
		return err
	}

	bufWriter := bufio.NewWriterSize(f, 64*1024)

	progressWriter := &progressWriter{
		total:      resp.ContentLength,
		fileName:   name,
		lastUpdate: time.Now(),
	}

	written, err := io.Copy(bufWriter, io.TeeReader(resp.Body, progressWriter))
	if err != nil {
		f.Close()
		os.Remove(partial)
		return err
	}

	if err := bufWriter.Flush(); err != nil {
		f.Close()
		os.Remove(partial)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(partial)
		return err
	}

	if written != resp.ContentLength && resp.ContentLength > 0 {
		os.Remove(partial)
		return fmt.Errorf("下载 %s 字节数不匹配 (期望: %d, 实际: %d)，已丢弃不完整文件", name, resp.ContentLength, written)
	}

	if err := os.Rename(partial, outfile); err != nil {
		return err
	}

	log.Printf("完成下载 %s", outfile)
	return nil
}

type progressWriter struct {
	total      int64
	written    int64
	fileName   string
	lastUpdate time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.total <= 0 {
		if time.Since(pw.lastUpdate) > 2*time.Second {
			pw.lastUpdate = time.Now()
			log.Printf("下载 %s: 已写入 %d 字节", pw.fileName, pw.written)
		}
		return n, nil
	}
	if time.Since(pw.lastUpdate) > 2*time.Second {
		pw.lastUpdate = time.Now()
		percentage := float64(pw.written) / float64(pw.total) * 100
		log.Printf("下载 %s: %d / %d (%.2f%%)", pw.fileName, pw.written, pw.total, percentage)
	}
	return n, nil
}

// FormatDownloadURL 构造对外展示的资产下载地址。
func FormatDownloadURL(serverAddress string, serverPort int, publicIP string, launcher, version, assetName string) string {
	var host string
	var scheme string = "http"

	if serverAddress != "" {
		host = serverAddress
		if strings.HasPrefix(serverAddress, "http://") {
			scheme = "http"
			host = strings.TrimPrefix(serverAddress, "http://")
		} else if strings.HasPrefix(serverAddress, "https://") {
			scheme = "https"
			host = strings.TrimPrefix(serverAddress, "https://")
		}
		// server_address 配置成 "example.com:8080" 时先剥掉端口，
		// 避免下方再拼一次端口产生 example.com:8080:8080。
		if h, _, err := net.SplitHostPort(strings.TrimSuffix(host, "/")); err == nil {
			host = h
		}
		host = strings.TrimSuffix(host, "/")
	} else {
		host = publicIP
	}

	// 如果端口不是 80/443，则格式化带端口的主机
	if serverPort != 80 && serverPort != 443 {
		host = fmt.Sprintf("%s:%d", host, serverPort)
	}

	return fmt.Sprintf("%s://%s/download/%s/%s/%s", scheme, host,
		url.PathEscape(launcher), url.PathEscape(version), url.PathEscape(assetName))
}
