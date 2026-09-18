package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v50/github"
)

func TestIsSafePathComponent(t *testing.T) {
	for _, name := range []string{"ok.zip", "1.2.3"} {
		if !isSafePathComponent(name) {
			t.Errorf("%q should be safe", name)
		}
	}
	for _, name := range []string{"", ".", "..", "../escape", `..\\escape`, "/tmp"} {
		if isSafePathComponent(name) {
			t.Errorf("%q should be rejected", name)
		}
	}
}

// newTestRelease 构造指向 httptest 服务器的单资产 release。
func newTestRelease(assetURL string, size int) *github.RepositoryRelease {
	return &github.RepositoryRelease{
		TagName: github.String("v1.0.0"),
		Assets: []*github.ReleaseAsset{{
			Name:               github.String("asset.txt"),
			Size:               github.Int(size),
			BrowserDownloadURL: github.String(assetURL),
		}}}
}

func readIndexAssets(t *testing.T, destBase string) []ReleaseAssetSimple {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(destBase, "demo", "v1.0.0", "index.json"))
	if err != nil {
		t.Fatalf("读取 index.json: %v", err)
	}
	var info ReleaseInfo
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatalf("解析 index.json: %v", err)
	}
	if len(info.Assets) != 1 {
		t.Fatalf("资产数量 = %d, 期望 1", len(info.Assets))
	}
	return info.Assets
}

// 下载后 index.json 必须带上与 GitHub digest 一致的 sha256。
func TestDownloadLatestWritesSHA256(t *testing.T) {
	content := []byte("hello sha256 asset payload")
	sum := sha256.Sum256(content)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(content)
	}))
	defer srv.Close()

	dest := t.TempDir()
	rel := newTestRelease(srv.URL+"/asset.txt", len(content))
	d := NewDownloader(1, 2, false)
	if _, err := d.DownloadLatest(context.Background(), "demo", dest, "", "", false, "", rel, "", 0, "http://test.example", true,
		map[string]string{"asset.txt": hex.EncodeToString(sum[:])}); err != nil {
		t.Fatalf("DownloadLatest: %v", err)
	}
	if got := readIndexAssets(t, dest)[0].SHA256; got != hex.EncodeToString(sum[:]) {
		t.Errorf("index.json sha256 = %q, 期望 %q", got, hex.EncodeToString(sum[:]))
	}
	if hits != 1 {
		t.Errorf("下载请求数 = %d, 期望 1", hits)
	}
}

// 本地已有文件且与 digest 一致时不重新下载，仅重算哈希。
func TestDownloadLatestExistingFileVerifiedWithoutRedownload(t *testing.T) {
	content := []byte("already on disk")
	sum := sha256.Sum256(content)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(content)
	}))
	defer srv.Close()

	dest := t.TempDir()
	versionDir := filepath.Join(dest, "demo", "v1.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "asset.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	rel := newTestRelease(srv.URL+"/asset.txt", len(content))
	d := NewDownloader(1, 2, false)
	if _, err := d.DownloadLatest(context.Background(), "demo", dest, "", "", false, "", rel, "", 0, "http://test.example", true,
		map[string]string{"asset.txt": hex.EncodeToString(sum[:])}); err != nil {
		t.Fatalf("DownloadLatest: %v", err)
	}
	if hits != 0 {
		t.Errorf("本地一致时不应发起下载，实际请求数 = %d", hits)
	}
	if got := readIndexAssets(t, dest)[0].SHA256; got != hex.EncodeToString(sum[:]) {
		t.Errorf("index.json sha256 = %q, 期望 %q", got, hex.EncodeToString(sum[:]))
	}
}

// 本地文件被篡改（大小不变）时：检测到 digest 不一致 → 重下 → 修复 → 元数据指向正确哈希。
func TestDownloadLatestRedownloadsOnDigestMismatch(t *testing.T) {
	good := []byte("good payload bytes!!!!!!")
	bad := []byte("bad payload bytes!!!!!!") // 同长度，内容不同
	sum := sha256.Sum256(good)
	served := bad
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(served)
		served = good // 首次返回篡改内容，重试后返回正确内容
	}))
	defer srv.Close()

	dest := t.TempDir()
	versionDir := filepath.Join(dest, "demo", "v1.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "asset.txt"), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	rel := newTestRelease(srv.URL+"/asset.txt", len(good))
	d := NewDownloader(1, 2, false)
	if _, err := d.DownloadLatest(context.Background(), "demo", dest, "", "", false, "", rel, "", 0, "http://test.example", true,
		map[string]string{"asset.txt": hex.EncodeToString(sum[:])}); err != nil {
		t.Fatalf("DownloadLatest: %v", err)
	}
	got := readIndexAssets(t, dest)[0].SHA256
	if got != hex.EncodeToString(sum[:]) {
		t.Errorf("篡改后未修复: index.json sha256 = %q, 期望 %q", got, hex.EncodeToString(sum[:]))
	}
	onDisk, err := os.ReadFile(filepath.Join(versionDir, "asset.txt"))
	if err != nil || string(onDisk) != string(good) {
		t.Errorf("落盘文件未修复为正确内容")
	}
}

// GitHub 始终与本地不一致时：两次重试后保留本地文件，元数据记录本地实际哈希（不报错）。
func TestDownloadLatestKeepsLocalHashOnPersistentMismatch(t *testing.T) {
	good := []byte("good payload bytes!!!!!!")
	bad := []byte("bad payload bytes!!!!!!")
	goodSum := sha256.Sum256(good)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bad)
	}))
	defer srv.Close()

	dest := t.TempDir()
	rel := newTestRelease(srv.URL+"/asset.txt", len(good))
	d := NewDownloader(1, 2, false)
	if _, err := d.DownloadLatest(context.Background(), "demo", dest, "", "", false, "", rel, "", 0, "http://test.example", true,
		map[string]string{"asset.txt": hex.EncodeToString(goodSum[:])}); err != nil {
		t.Fatalf("DownloadLatest 不应报错: %v", err)
	}
	badSum := sha256.Sum256(bad)
	if got := readIndexAssets(t, dest)[0].SHA256; got != hex.EncodeToString(badSum[:]) {
		t.Errorf("元数据应与落盘文件一致: got %q, 期望 %q", got, hex.EncodeToString(badSum[:]))
	}
}

// GitHub 未提供 digest（旧资产）时：仅记录本地计算哈希。
func TestDownloadLatestWithoutDigestRecordsLocalHash(t *testing.T) {
	content := []byte("no digest provided")
	sum := sha256.Sum256(content)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dest := t.TempDir()
	rel := newTestRelease(srv.URL+"/asset.txt", len(content))
	d := NewDownloader(1, 2, false)
	if _, err := d.DownloadLatest(context.Background(), "demo", dest, "", "", false, "", rel, "", 0, "http://test.example", true, nil); err != nil {
		t.Fatalf("DownloadLatest: %v", err)
	}
	if got := readIndexAssets(t, dest)[0].SHA256; got != hex.EncodeToString(sum[:]) {
		t.Errorf("index.json sha256 = %q, 期望 %q", got, hex.EncodeToString(sum[:]))
	}
}
