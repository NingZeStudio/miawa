package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lemwood_mirror/internal/config"
)

// 复现 zl2/2.5.2 事故的姊妹篇：带宽压力分流。landing 端点在主服活跃下载
// 连接数达到阈值时，把名单内启动器的浏览器下载引导到备用节点验证页；
// 名单外/未达阈值/未启用时保持本机下载 URL。
func setupLandingOffloadState(t *testing.T, mutate func(*config.Config)) (*State, http.Handler) {
	t.Helper()
	cfg := &config.Config{
		PowEnabled:               false,
		BandwidthLimitMbps:       100,
		DownloadOffloadURL:       "https://us1.miawa.cn",
		DownloadOffloadActive:    1,
		DownloadOffloadLaunchers: []string{"zl2", "axolotl"},
	}
	if mutate != nil {
		mutate(cfg)
	}
	state, handler, _ := setupDownloadHandlerState(t, cfg, 0, rangeTestContent)

	// prepare/landing 校验文件存在，补建分流测试用的启动器文件
	for _, p := range []string{"zl2/v2.5.3/app.apk", "hmcl/v3.16.3/app.exe"} {
		full := filepath.Join(state.BasePath, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	return state, handler
}

func landingDownloadURL(t *testing.T, handler http.Handler, token string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/downloads/landing?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("landing status = %d, body = %s", rec.Code, rec.Body.String())
	}
	resp := unwrapV2Envelope(t, rec.Body.Bytes())
	return resp
}

func TestLandingOffloadsUnderBandwidthPressure(t *testing.T) {
	state, handler := setupLandingOffloadState(t, nil)
	token := prepareToken(t, handler, "zl2/v2.5.3/app.apk")

	state.bandwidth.StartDownload() // 1 个活跃下载 ≥ 阈值 1
	defer state.bandwidth.FinishDownload()

	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	want := "https://us1.miawa.cn/verify?file=zl2%2Fv2.5.3%2Fapp.apk"
	if got != want {
		t.Fatalf("download_url = %s, want %s", got, want)
	}
	if offloaded, _ := resp["offloaded"].(bool); !offloaded {
		t.Fatal("offloaded 应为 true")
	}
}

func TestLandingOffloadCarriesReturnURL(t *testing.T) {
	state, handler := setupLandingOffloadState(t, nil)

	body := `{"file_path":"zl2/v2.5.3/app.apk","source":"test","return_url":"/files?launcher=zl2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/downloads/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare status = %d, body = %s", rec.Code, rec.Body.String())
	}
	prepareResp := unwrapV2Envelope(t, rec.Body.Bytes())
	token, _ := prepareResp["download_token"].(string)
	if token == "" {
		t.Fatalf("download_token 为空: %v", prepareResp)
	}

	state.bandwidth.StartDownload()
	defer state.bandwidth.FinishDownload()

	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	if !strings.Contains(got, "return_url=") {
		t.Fatalf("分流 URL 应携带 return_url，got %s", got)
	}
}

func TestLandingStaysLocalBelowThreshold(t *testing.T) {
	_, handler := setupLandingOffloadState(t, nil)
	token := prepareToken(t, handler, "zl2/v2.5.3/app.apk")

	// 无活跃下载，低于阈值
	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	if got != "/download/zl2/v2.5.3/app.apk?token="+token {
		t.Fatalf("download_url = %s, want 本机地址", got)
	}
	if offloaded, _ := resp["offloaded"].(bool); offloaded {
		t.Fatal("未达阈值不应标记 offloaded")
	}
}

func TestLandingNoOffloadForLauncherOutsideList(t *testing.T) {
	state, handler := setupLandingOffloadState(t, nil)
	token := prepareToken(t, handler, "hmcl/v3.16.3/app.exe")

	state.bandwidth.StartDownload()
	defer state.bandwidth.FinishDownload()

	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	if got != "/download/hmcl/v3.16.3/app.exe?token="+token {
		t.Fatalf("名单外启动器不应分流，download_url = %s", got)
	}
}

func TestLandingNoOffloadWhenDisabled(t *testing.T) {
	state, handler := setupLandingOffloadState(t, func(cfg *config.Config) {
		cfg.DownloadOffloadURL = ""
		cfg.DownloadOffloadActive = 0
	})
	token := prepareToken(t, handler, "zl2/v2.5.3/app.apk")

	state.bandwidth.StartDownload()
	defer state.bandwidth.FinishDownload()

	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	if got != "/download/zl2/v2.5.3/app.apk?token="+token {
		t.Fatalf("未启用分流时不应改变 download_url，got %s", got)
	}
}

// 带宽水位触发：实时带宽达到 download_offload_mbps 阈值即分流（活跃连接数
// 阈值置 0 禁用，单独验证带宽信号）。
func TestLandingOffloadsOnBandwidthThreshold(t *testing.T) {
	state, handler := setupLandingOffloadState(t, func(cfg *config.Config) {
		cfg.DownloadOffloadActive = 0
		cfg.DownloadOffloadMbps = 1
	})
	token := prepareToken(t, handler, "zl2/v2.5.3/app.apk")

	// 10MB 写入 10 秒测量窗口 ≈ 8 Mbps ≥ 阈值 1
	state.bandwidth.RecordBytes(10 << 20)

	resp := landingDownloadURL(t, handler, token)
	got, _ := resp["download_url"].(string)
	want := "https://us1.miawa.cn/verify?file=zl2%2Fv2.5.3%2Fapp.apk"
	if got != want {
		t.Fatalf("带宽超阈值应分流，download_url = %s", got)
	}
}
