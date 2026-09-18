package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lemwood_mirror/internal/config"
)

func TestFileIntegrityEndpoint(t *testing.T) {
	cfg := &config.Config{PowEnabled: false, AppealContact: "test-contact"}
	content := "hello integrity"
	_, handler, _ := setupDownloadHandlerState(t, cfg, 1, content)

	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])

	req := httptest.NewRequest(http.MethodGet, "/api/v2/files/integrity?file_path=launcher/v1/file.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			FilePath  string `json:"file_path"`
			FileName  string `json:"file_name"`
			Size      int64  `json:"size"`
			Algorithm string `json:"algorithm"`
			Sha256    string `json:"sha256"`
		} `json:"data"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Error != nil {
		t.Fatalf("error field = %v", body.Error)
	}
	if body.Data.Sha256 != want {
		t.Fatalf("sha256 = %s, want %s", body.Data.Sha256, want)
	}
	if body.Data.FileName != "file.txt" {
		t.Fatalf("file_name = %s, want file.txt", body.Data.FileName)
	}
	if body.Data.Algorithm != "sha256" {
		t.Fatalf("algorithm = %s, want sha256", body.Data.Algorithm)
	}
	if body.Data.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", body.Data.Size, len(content))
	}

	// 缓存命中路径：第二次请求结果一致
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), want) {
		t.Fatalf("second response missing hash: %s", rec2.Body.String())
	}

	// 缺少 file_path 参数
	req3 := httptest.NewRequest(http.MethodGet, "/api/v2/files/integrity", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("missing param status = %d, want 400", rec3.Code)
	}

	// 路径逃逸被拒
	req4 := httptest.NewRequest(http.MethodGet, "/api/v2/files/integrity?file_path=../../etc/passwd", nil)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code == http.StatusOK {
		t.Fatalf("path traversal should not succeed: %s", rec4.Body.String())
	}

	// 不存在的文件
	req5 := httptest.NewRequest(http.MethodGet, "/api/v2/files/integrity?file_path=launcher/v1/missing.txt", nil)
	rec5 := httptest.NewRecorder()
	handler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusNotFound {
		t.Fatalf("missing file status = %d, want 404", rec5.Code)
	}
}

func TestDownloadServesSHA256Header(t *testing.T) {
	cfg := &config.Config{PowEnabled: false, AppealContact: "test-contact"}
	content := "hello integrity header"
	state, _, _ := setupDownloadHandlerState(t, cfg, 1, content)

	// 直接签发授权（走 issueAuthz），再用 token 请求下载，校验 X-Content-SHA256 头
	resp, err := state.issueAuthz("launcher/v1/file.txt", "", "home", "prepare", "192.0.2.10", "api", int64(len(content)))
	if err != nil {
		t.Fatalf("issueAuthz() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/download/launcher/v1/file.txt?token="+resp.DownloadToken, nil)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	state.Routes(mux)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])
	if got := rec.Header().Get("X-Content-SHA256"); got != want {
		t.Fatalf("X-Content-SHA256 = %s, want %s", got, want)
	}
}
