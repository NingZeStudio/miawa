package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gh "lemwood_mirror/internal/github"
)

// fakeUpstream 是一个同时扮演 GitHub API 与 Release 下载源的本地假上游。
// GitHub API 通过 NewClientWithBaseURL 直连本地服务器；更新包下载通过把
// asset_proxy_url 配置为镜像前缀（<server>/mirror/）路由到本地服务器，
// 从而在不访问真实 github.com 的前提下跑通 Check → fetchAssetDigests →
// Apply 全链路（https 目标经 HTTP 代理会走 CONNECT，无法用明文假上游模拟）。
type fakeUpstream struct {
	mu sync.Mutex

	tag       string
	rawBody   []byte // 裸二进制资产内容；nil 表示 404（触发压缩包回退）
	targzBody []byte // 压缩包资产内容；nil 表示 404
	digests   map[string]string

	assetRequests []string
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	f.mu.Lock()
	f.assetRequests = append(f.assetRequests, p)
	digests := f.digests
	rawBody, targzBody, tag := f.rawBody, f.targzBody, f.tag
	f.mu.Unlock()

	// 镜像前缀路径形如 /mirror/https://github.com/<owner>/<repo>/releases/download/<tag>/<asset>
	if i := strings.Index(p, "/releases/download/"+tag+"/"); i >= 0 {
		asset := strings.TrimPrefix(p[i+len("/releases/download/"+tag+"/"):], "/")
		switch asset {
		case platformAssetName():
			if rawBody == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(rawBody)
		case platformArchiveName():
			if targzBody == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(targzBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	switch {
	case strings.HasSuffix(p, "/repos/testowner/testrepo/tags"):
		writeJSON(w, []map[string]string{{"name": tag}})
	case strings.HasSuffix(p, "/repos/testowner/testrepo/releases/tags/"+tag):
		assets := make([]map[string]string, 0, len(digests))
		for name, digest := range digests {
			assets = append(assets, map[string]string{"name": name, "digest": "sha256:" + digest})
		}
		writeJSON(w, map[string]any{"assets": assets})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func elfBinary(payload string) []byte {
	return append([]byte{0x7f, 0x45, 0x4c, 0x46}, payload...) // ELF 魔数 + 内容
}

func sha256HexOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func buildTargz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar WriteHeader() error = %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar Write() error = %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return buf.Bytes()
}

// newE2EManager 启动假上游并创建指向它的 Manager，返回 (manager, 上游, 目标二进制路径)。
func newE2EManager(t *testing.T, currentVersion string, upstream *fakeUpstream) (*Manager, *fakeUpstream, string) {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)

	target := filepath.Join(t.TempDir(), "mirror")
	oldBinary := append([]byte(nil), elfBinary("old-version")...)
	if err := os.WriteFile(target, oldBinary, 0o755); err != nil {
		t.Fatalf("写旧二进制失败: %v", err)
	}

	cfg := Config{
		Enabled:       true,
		RepoURL:       "https://github.com/testowner/testrepo",
		Channel:       string(ChannelRelease),
		AutoRestart:   false,
		AssetProxyURL: server.URL + "/mirror/", // 镜像前缀：下载路由到假上游
	}
	m := NewManager(gh.NewClientWithBaseURL("", "", server.URL+"/"), currentVersion, target, cfg)
	return m, upstream, target
}

// TestSelfUpdateApplyEndToEnd 全链路：Check 发现新版本 → 取资产摘要 →
// 下载裸二进制 → SHA-256 校验 → 替换本地二进制。
func TestSelfUpdateApplyEndToEnd(t *testing.T) {
	bin := elfBinary("updated-payload")
	upstream := &fakeUpstream{
		tag:     "9.9.9",
		rawBody: bin,
		digests: map[string]string{platformAssetName(): sha256HexOf(bin)},
	}
	m, _, target := newE2EManager(t, "1.0.0", upstream)

	status, err := m.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.HasUpdate || status.LatestVersion != "9.9.9" {
		t.Fatalf("Check() 未发现更新: latest=%q hasUpdate=%v", status.LatestVersion, status.HasUpdate)
	}
	if !status.CanApply {
		t.Fatalf("Check() CanApply = false, 期望 release 通道可应用")
	}

	status, err = m.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	// AutoRestart=false 时不会自动重启，但安装完成后仍处于"待重启"状态
	if !status.PendingRestart {
		t.Fatalf("安装完成后 PendingRestart 应为 true（等待重启生效）")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取更新后二进制失败: %v", err)
	}
	if !bytes.Equal(got, bin) {
		t.Fatalf("更新后二进制内容不匹配: got %d bytes, want %d bytes", len(got), len(bin))
	}
	if status.LastApplyError != "" {
		t.Fatalf("LastApplyError = %q, 期望为空", status.LastApplyError)
	}
	if !strings.Contains(status.LastApplyMessage, platformAssetName()) {
		t.Fatalf("LastApplyMessage = %q, 应包含已安装资产名", status.LastApplyMessage)
	}
}

// TestSelfUpdateApplyDigestMismatch 摘要不匹配必须中止：二进制不被替换。
func TestSelfUpdateApplyDigestMismatch(t *testing.T) {
	bin := elfBinary("tampered-payload")
	upstream := &fakeUpstream{
		tag:     "9.9.9",
		rawBody: bin,
		digests: map[string]string{
			platformAssetName():   sha256HexOf([]byte("other-bytes")), // 错误摘要
			platformArchiveName(): sha256HexOf([]byte("other-bytes")),
		},
	}
	m, _, target := newE2EManager(t, "1.0.0", upstream)

	if _, err := m.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	_, err := m.Apply(context.Background())
	if err == nil {
		t.Fatalf("Apply() 应因摘要不匹配失败")
	}
	if !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("Apply() 错误应包含 SHA-256, got: %v", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("读取目标二进制失败: %v", readErr)
	}
	if want := elfBinary("old-version"); !bytes.Equal(got, want) {
		t.Fatalf("校验失败后目标二进制不应被替换")
	}
}

// TestSelfUpdateApplyArchiveFallback 裸二进制 404 时回退 tar.gz 压缩包并解压替换。
func TestSelfUpdateApplyArchiveFallback(t *testing.T) {
	bin := elfBinary("archived-payload")
	upstream := &fakeUpstream{
		tag:       "9.9.9",
		rawBody:   nil, // 404
		targzBody: buildTargz(t, platformAssetName(), bin),
		digests:   map[string]string{platformArchiveName(): sha256HexOf(upstreamArchiveBytes(t, bin))},
	}
	m, _, target := newE2EManager(t, "1.0.0", upstream)

	if _, err := m.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if _, err := m.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取更新后二进制失败: %v", err)
	}
	if !bytes.Equal(got, bin) {
		t.Fatalf("压缩包回退更新后内容不匹配: got %d bytes, want %d bytes", len(got), len(bin))
	}
}

// upstreamArchiveBytes 计算 tar.gz 字节（与 buildTargz 相同流程，供摘要预计算）。
func upstreamArchiveBytes(t *testing.T, content []byte) []byte {
	t.Helper()
	return buildTargz(t, platformAssetName(), content)
}

// TestSelfUpdateCheckErrorSurfaces 上游不可用时 Check 返回错误并记录 LastCheckError。
func TestSelfUpdateCheckErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "mirror")
	_ = os.WriteFile(target, elfBinary("old"), 0o755)
	m := NewManager(
		gh.NewClientWithBaseURL("", "", server.URL+"/"),
		"1.0.0",
		target,
		Config{Enabled: true, RepoURL: "https://github.com/testowner/testrepo", Channel: string(ChannelRelease)},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Check(ctx); err == nil {
		t.Fatalf("Check() 应因上游 500 失败")
	}
	status := m.Status()
	if status.LastCheckError == "" {
		t.Fatalf("LastCheckError 应被记录")
	}
	if status.CanApply {
		t.Fatalf("检查失败后 CanApply 应为 false")
	}
}

// TestSelfUpdateNotifyChannelCannotApply notify 通道只提示不应用。
func TestSelfUpdateNotifyChannelCannotApply(t *testing.T) {
	upstream := &fakeUpstream{
		tag:     "9.9.9",
		rawBody: elfBinary("updated"),
		digests: map[string]string{},
	}
	m, _, _ := newE2EManager(t, "1.0.0", upstream)
	// 重写通道为 notify
	m.mu.Lock()
	m.status.Channel = string(ChannelNotify)
	m.mu.Unlock()

	status, err := m.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !status.HasUpdate {
		t.Fatalf("notify 通道也应发现更新")
	}
	if status.CanApply {
		t.Fatalf("notify 通道 CanApply 应为 false")
	}
	if _, err := m.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "不可应用") {
		t.Fatalf("notify 通道 Apply 应被拒绝, got: %v", err)
	}
}
