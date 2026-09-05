package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
)

// prepareToken 通过 prepare 端点签发一个下载 token（CLI/API 直发授权）。
func prepareToken(t *testing.T, handler http.Handler, filePath string) string {
	t.Helper()
	body := fmt.Sprintf(`{"file_path":%q,"source":"test"}`, filePath)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/downloads/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare status = %d, body = %s", rec.Code, rec.Body.String())
	}
	resp := unwrapV2Envelope(t, rec.Body.Bytes())
	token, _ := resp["download_token"].(string)
	if token == "" {
		t.Fatalf("download_token 为空: %v", resp)
	}
	return token
}

const rangeTestContent = "0123456789abcdefghij"

func newRangeTestState(t *testing.T) (http.Handler, string) {
	t.Helper()
	cfg := &config.Config{
		PowEnabled:    false,
		AppealContact: "test-contact",
	}
	_, handler, dlPath := setupDownloadHandlerState(t, cfg, 0, rangeTestContent)
	return handler, dlPath
}

// TestDownloadParallelSegmentsReuseToken 多线程下载器场景：第一条连接消费
// 一次性授权后，同一 IP 对同一文件的后续分段连接应凭同一 token 复用放行。
func TestDownloadParallelSegmentsReuseToken(t *testing.T) {
	handler, dlPath := newRangeTestState(t)
	token := prepareToken(t, handler, "launcher/v1/file.txt")

	// 连接 1：整文件下载（消费授权）
	req := httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首段 status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 连接 2：Range 分段（复用已消费 token）
	req = httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	req.Header.Set("Range", "bytes=0-4")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("分段 status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "01234" {
		t.Fatalf("分段内容 = %q, want %q", got, "01234")
	}

	// 连接 3：另一段（并发复用）
	req = httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	req.Header.Set("Range", "bytes=10-14")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("并发分段 status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != "abcde" {
		t.Fatalf("并发分段内容 = %q, want %q", got, "abcde")
	}
}

// TestDownloadRangeOnFreshToken 首次请求即带 Range：206 且只传分段。
func TestDownloadRangeOnFreshToken(t *testing.T) {
	handler, dlPath := newRangeTestState(t)
	token := prepareToken(t, handler, "launcher/v1/file.txt")

	req := httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	req.Header.Set("Range", "bytes=5-9")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != "56789" {
		t.Fatalf("内容 = %q, want %q", got, "56789")
	}
}

// TestDownloadReuseRejectsDifferentFile 复用窗口内 token 换文件仍被拒绝。
func TestDownloadReuseRejectsDifferentFile(t *testing.T) {
	cfg := &config.Config{PowEnabled: false, AppealContact: "test-contact"}
	state, handler, _ := setupDownloadHandlerState(t, cfg, 0, rangeTestContent)
	// 在同一存储目录下再造一个文件（setup 只建了 file.txt）
	otherPath := filepath.Join(state.BasePath, "launcher", "v1", "other.txt")
	if err := os.WriteFile(otherPath, []byte(rangeTestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	token := prepareToken(t, handler, "launcher/v1/file.txt")
	req := httptest.NewRequest(http.MethodGet, "/download/launcher/v1/file.txt?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次下载 status = %d", rec.Code)
	}

	// 真实分段场景：换文件请求同样携带 Range 头
	req = httptest.NewRequest(http.MethodGet, "/download/launcher/v1/other.txt?token="+token, nil)
	req.Header.Set("Range", "bytes=0-4")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("换文件复用 status = %d, want 403", rec.Code)
	}
	code, _ := unwrapV2ErrorNonFatal(t, rec.Body.Bytes())
	if code != "token_mismatch" {
		t.Fatalf("error code = %q, want token_mismatch", code)
	}
}

// TestDownloadReuseRejectsDifferentIP 复用绑定签发时的客户端 IP。
func TestDownloadReuseRejectsDifferentIP(t *testing.T) {
	handler, dlPath := newRangeTestState(t)
	token := prepareToken(t, handler, "launcher/v1/file.txt")

	req := httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次下载 status = %d", rec.Code)
	}

	// 换一个客户端 IP 复用
	req = httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	req.RemoteAddr = "203.0.113.77:4444"
	req.Header.Set("Range", "bytes=0-4")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("跨 IP 复用 status = %d, want 403", rec.Code)
	}
	code, _ := unwrapV2ErrorNonFatal(t, rec.Body.Bytes())
	if code != "client_ip_mismatch" {
		t.Fatalf("error code = %q, want client_ip_mismatch", code)
	}
}

// TestDownloadReuseExpiresAfterTTL 复用窗口不超过授权 TTL，过期后拒绝。
func TestDownloadReuseExpiresAfterTTL(t *testing.T) {
	handler, dlPath := newRangeTestState(t)
	token := prepareToken(t, handler, "launcher/v1/file.txt")

	req := httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次下载 status = %d", rec.Code)
	}

	// 把 consumed_at 拨回到 TTL 之前（默认 5m）
	stale := time.Now().UTC().Add(-10 * time.Minute).Format(db.AuthzTimeFormat)
	if _, err := db.DB.Exec(`UPDATE download_authorizations SET consumed_at=?`, stale); err != nil {
		t.Fatalf("拨回 consumed_at 失败: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, dlPath+"?token="+token, nil)
	req.Header.Set("Range", "bytes=0-4")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("TTL 过期后复用 status = %d, want 403", rec.Code)
	}
}

// unwrapV2ErrorNonFatal 解包 v2 错误信封的 error.code（不要求必有错误）。
func unwrapV2ErrorNonFatal(t *testing.T, body []byte) (string, string) {
	t.Helper()
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("Unmarshal error = %v, body = %s", err, string(body))
	}
	return env.Error, env.Message
}
