package blacklist

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/firewall"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	externalIPs   map[string]bool
	externalIPsMu sync.RWMutex
	lastSyncTime  time.Time
	syncMu        sync.Mutex
)

func init() {
	externalIPs = make(map[string]bool)
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

func SyncExternalBlacklist(url string, insecureSkipVerify bool) error {
	if url == "" {
		return nil
	}

	syncMu.Lock()
	defer syncMu.Unlock()

	var transport http.RoundTripper = http.DefaultTransport
	if insecureSkipVerify {
		transport = insecureTransport()
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取外部黑名单失败，状态码: %d", resp.StatusCode)
	}

	var ips []string
	var newExternalIPs = make(map[string]bool)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ip := parseBlacklistLine(line)
		if ip == "" {
			continue
		}
		ips = append(ips, ip)
		newExternalIPs[ip] = true
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}

	externalIPsMu.Lock()
	externalIPs = newExternalIPs
	lastSyncTime = time.Now()
	externalIPsMu.Unlock()

	if err := db.AddExternalBlacklist(ips); err != nil {
		return err
	}

	// 外部源可能包含 CIDR 网段条目，刷新内存网段集合
	if err := firewall.RefreshBlacklist(); err != nil {
		log.Printf("[黑名单同步] 刷新网段黑名单失败: %v", err)
	}

	log.Printf("[黑名单同步] 成功同步 %d 个外部黑名单IP", len(ips))
	return nil
}

func IsExternalBlacklisted(ip string) bool {
	externalIPsMu.RLock()
	defer externalIPsMu.RUnlock()
	return externalIPs[ip]
}

// parseBlacklistLine 从外部黑名单行提取 IP。
// 兼容格式：纯 IP、行内 # 注释、"ip:port"（仅当冒号唯一时才按 host:port 处理，
// 避免破坏 IPv6 地址自身的多个冒号）。无法解析为合法 IP 的行直接丢弃。
func parseBlacklistLine(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	// 取第一个空白分隔字段
	if fields := strings.Fields(line); len(fields) > 0 {
		line = fields[0]
	}
	if net.ParseIP(line) != nil {
		return line
	}
	// 唯一一个冒号才可能是 ip:port；IPv6 会有多个冒号，不能拆
	if strings.Count(line, ":") == 1 {
		if host, _, err := net.SplitHostPort(line); err == nil && net.ParseIP(host) != nil {
			return host
		}
	}
	return ""
}

func GetExternalBlacklistCount() int {
	externalIPsMu.RLock()
	defer externalIPsMu.RUnlock()
	return len(externalIPs)
}

func GetLastSyncTime() time.Time {
	externalIPsMu.RLock()
	defer externalIPsMu.RUnlock()
	return lastSyncTime
}
