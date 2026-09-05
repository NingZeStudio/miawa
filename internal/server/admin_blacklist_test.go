package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"lemwood_mirror/internal/auth"
	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/firewall"
)

// setupAdminTest 建立带管理员认证的测试环境，返回 handler 与 Authorization 头。
func setupAdminTest(t *testing.T) (http.Handler, string) {
	t.Helper()
	cfg := &config.Config{PowEnabled: false, AppealContact: "test-contact", AdminEnabled: true}
	_, handler, _ := setupDownloadHandlerState(t, cfg, 0, "hello")

	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	return handler, token
}

func adminGet(handler http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func seedBlacklist(t *testing.T) {
	t.Helper()
	entries := []struct{ ip, source, banType string }{
		{"192.0.2.101", "manual", "manual"},
		{"192.0.2.22", "local", "traffic"},
		{"192.0.2.33", "local", "rate_limit"},
		{"198.51.100.7", "external", "manual"},
		{"203.0.113.0/24", "manual", "manual"},
		{"203.0.113.99", "local", "traffic"},
	}
	for i, e := range entries {
		reason := fmt.Sprintf("原因-%d 关键词%d", i, i)
		if err := db.AddIPToBlacklistWithSource(e.ip, reason, e.source, e.banType); err != nil {
			t.Fatalf("seed %s: %v", e.ip, err)
		}
	}
}

// TestAdminBlacklistPaged 服务端分页：page/page_size/source/q 过滤与统计。
func TestAdminBlacklistPaged(t *testing.T) {
	handler, token := setupAdminTest(t)
	seedBlacklist(t)

	// 第 1 页，每页 2 条
	rec := adminGet(handler, "/api/v2/admin/blacklist?page=1&page_size=2", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Items    []map[string]string `json:"items"`
			Total    int                 `json:"total"`
			Page     int                 `json:"page"`
			PageSize int                 `json:"page_size"`
			Stats    map[string]int      `json:"stats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if env.Data.Total != 6 || len(env.Data.Items) != 2 || env.Data.Page != 1 || env.Data.PageSize != 2 {
		t.Fatalf("分页结果不符: total=%d items=%d page=%d size=%d", env.Data.Total, len(env.Data.Items), env.Data.Page, env.Data.PageSize)
	}
	if env.Data.Stats["manual"] != 2 || env.Data.Stats["external"] != 1 || env.Data.Stats["local"] != 3 || env.Data.Stats["all"] != 6 {
		t.Fatalf("统计不符: %v", env.Data.Stats)
	}
	if env.Data.Stats["auto"] != 3 {
		t.Fatalf("前端 auto 统计（local+auto）应为 3: %v", env.Data.Stats)
	}

	// source=local 过滤（自动封禁，含历史 auto 值）
	rec = adminGet(handler, "/api/v2/admin/blacklist?page=1&page_size=50&source=local", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("source 过滤 status = %d", rec.Code)
	}
	var envLocal struct {
		Data struct {
			Items []map[string]string `json:"items"`
			Total int                 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envLocal); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if envLocal.Data.Total != 3 || len(envLocal.Data.Items) != 3 {
		t.Fatalf("source=local 应有 3 条: total=%d items=%d", envLocal.Data.Total, len(envLocal.Data.Items))
	}

	// 关键词过滤（按 ip 字面匹配，下划线不是通配符）
	rec = adminGet(handler, fmt.Sprintf("/api/v2/admin/blacklist?page=1&page_size=50&q=%s", "192.0.2.2_"), token)
	var envKw struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envKw); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if envKw.Data.Total != 0 {
		t.Fatalf("字面关键词 192.0.2.2_ 不应命中（_ 不是通配符）: total=%d", envKw.Data.Total)
	}

	// 无 page 参数：旧版全量列表行为
	rec = adminGet(handler, "/api/v2/admin/blacklist", token)
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &struct {
		Data *[]map[string]any `json:"data"`
	}{Data: &list}); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if len(list) != 6 {
		t.Fatalf("旧版全量应为 6 条: %d", len(list))
	}
}

// TestAdminBlacklistPagedRequiresAuth 未认证访问返回 401。
func TestAdminBlacklistPagedRequiresAuth(t *testing.T) {
	handler, _ := setupAdminTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/admin/blacklist?page=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestAdminFirewallStatusEndpoint 防火墙状态端点返回设置与内存计数。
func TestAdminFirewallStatusEndpoint(t *testing.T) {
	handler, token := setupAdminTest(t)

	// 白名单含测试默认 RemoteAddr（192.0.2.1），管理请求自身不计入活跃窗口
	firewall.Init(firewall.Settings{Enabled: true, PerMinute: 123, BanThreshold: 4},
		[]string{"192.0.2.1", "10.0.0.1", "10.0.0.0/8"}, nil)
	t.Cleanup(firewall.Close)
	// 触发一次请求产生活跃窗口
	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	req.RemoteAddr = "203.0.113.55:9999"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	rec = adminGet(handler, "/api/v2/admin/firewall/status", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Settings struct {
				Enabled      bool `json:"enabled"`
				PerMinute    int  `json:"per_minute"`
				BanThreshold int  `json:"ban_threshold"`
			} `json:"settings"`
			WhitelistCount int `json:"whitelist_count"`
			TrackedIPs     int `json:"tracked_ips"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !env.Data.Settings.Enabled || env.Data.Settings.PerMinute != 123 || env.Data.Settings.BanThreshold != 4 {
		t.Fatalf("设置不符: %+v", env.Data.Settings)
	}
	if env.Data.WhitelistCount != 3 {
		t.Fatalf("白名单计数 = %d, want 3", env.Data.WhitelistCount)
	}
	if env.Data.TrackedIPs != 1 {
		t.Fatalf("活跃窗口 IP 数 = %d, want 1", env.Data.TrackedIPs)
	}
}
