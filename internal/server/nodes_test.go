package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lemwood_mirror/internal/auth"
	"lemwood_mirror/internal/config"
)

// 节点互联契约：/api/v2/node/* 必须 X-Node-Key 鉴权（缺失/错误一律 403）；
// 主服节点轮询把子节点状态聚合进缓存并注入 /api/v2/stats；
// 管理端节点清单 CRUD 做 URL 校验并持久化。

func setupNodesState(t *testing.T, mutate func(*config.Config)) (*State, http.Handler) {
	t.Helper()
	cfg := &config.Config{
		PowEnabled:         false,
		BandwidthLimitMbps: 100,
		NodeName:           "edge-test",
		NodeAPIKey:         "node-secret",
	}
	if mutate != nil {
		mutate(cfg)
	}
	state, handler, _ := setupDownloadHandlerState(t, cfg, 0, rangeTestContent)
	return state, handler
}

func TestNodeStatusRequiresKey(t *testing.T) {
	_, handler := setupNodesState(t, nil)

	cases := map[string]*http.Request{
		"无密钥":  httptest.NewRequest(http.MethodGet, "/api/v2/node/status", nil),
		"错误密钥": withNodeKey(httptest.NewRequest(http.MethodGet, "/api/v2/node/status", nil), "wrong"),
	}
	for name, req := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", name, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, withNodeKey(httptest.NewRequest(http.MethodGet, "/api/v2/node/status", nil), "node-secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("正确密钥 status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if env.Data["node_name"] != "edge-test" {
		t.Fatalf("node_name = %v, want edge-test", env.Data["node_name"])
	}
	if _, ok := env.Data["launchers"].([]any); !ok {
		t.Fatal("launchers 应为数组")
	}
}

func TestNodeBlacklistRequiresKey(t *testing.T) {
	_, handler := setupNodesState(t, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/node/blacklist", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("无密钥 status = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, withNodeKey(httptest.NewRequest(http.MethodGet, "/api/v2/node/blacklist", nil), "node-secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("正确密钥 status = %d", rec.Code)
	}
}

// enableAdmin 打开管理后台（admin 中间件要求 AdminEnabled + 凭据非空）。
func enableAdmin(cfg *config.Config) {
	cfg.AdminEnabled = true
	cfg.AdminUser = "admin"
	hashed, err := auth.HashPassword("test-password")
	if err != nil {
		panic(err)
	}
	cfg.AdminPassword = hashed
}

func withNodeKey(req *http.Request, key string) *http.Request {
	req.Header.Set("X-Node-Key", key)
	return req
}

func adminRequest(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPollNodesAggregatesStatus(t *testing.T) {
	var gotKey string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Node-Key")
		if r.URL.Path != "/api/v2/node/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"node_name":        "us1",
				"version":          "1.3.3",
				"uptime_seconds":   120,
				"active_downloads": 3,
				"launchers":        []map[string]any{{"name": "zl2", "versions": 3, "latest": "2.5.3"}},
			},
		})
	}))
	t.Cleanup(fake.Close)

	state, _ := setupNodesState(t, func(cfg *config.Config) {
		cfg.MirrorNodes = []config.NodeRoute{{Name: "us1", URL: fake.URL}}
	})

	state.PollNodes(context.Background())
	if gotKey != "node-secret" {
		t.Fatalf("轮询应携带 X-Node-Key, got %q", gotKey)
	}

	snap := state.nodeStatusSnapshot()
	if len(snap) != 1 || !snap[0].Online {
		t.Fatalf("nodeStatusSnapshot = %+v, want 1 个在线节点", snap)
	}
	if snap[0].NodeName != "us1" || snap[0].Version != "1.3.3" || snap[0].ActiveDownloads != 3 {
		t.Fatalf("聚合字段不符: %+v", snap[0])
	}
	if len(snap[0].Launchers) != 1 || snap[0].Launchers[0].Latest != "2.5.3" {
		t.Fatalf("launchers 不符: %+v", snap[0].Launchers)
	}
}

func TestPollNodesMarksOfflineOnError(t *testing.T) {
	state, _ := setupNodesState(t, func(cfg *config.Config) {
		cfg.MirrorNodes = []config.NodeRoute{{Name: "dead", URL: "http://127.0.0.1:1"}}
	})
	state.PollNodes(context.Background())
	snap := state.nodeStatusSnapshot()
	if len(snap) != 1 || snap[0].Online {
		t.Fatalf("不可达节点应标记 offline: %+v", snap)
	}
	if snap[0].Error == "" {
		t.Fatal("offline 节点应保留错误信息")
	}
}

func TestStatsResponseIncludesNodes(t *testing.T) {
	state, _ := setupNodesState(t, func(cfg *config.Config) {
		cfg.MirrorNodes = []config.NodeRoute{{Name: "us1", URL: "http://127.0.0.1:1"}}
	})
	state.PollNodes(context.Background())

	mux := http.NewServeMux()
	state.Routes(mux)
	handler := state.SecurityMiddleware(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", rec.Code)
	}
	var env struct {
		Data struct {
			Nodes []NodeStatus `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if len(env.Data.Nodes) != 1 || env.Data.Nodes[0].Name != "us1" {
		t.Fatalf("stats.nodes = %+v, want 含 us1", env.Data.Nodes)
	}
}

func TestAdminNodesSaveAndValidate(t *testing.T) {
	_, handler := setupNodesState(t, func(cfg *config.Config) {
		enableAdmin(cfg)
		cfg.StoragePath = "storage" // NormalizeConfig 必填校验
	})
	token := mustAdminToken(t)

	// 非法 URL 被校验拒绝
	rec := adminRequest(t, handler, token, http.MethodPost, "/api/v2/admin/nodes",
		`{"nodes":[{"name":"bad","url":"ftp://x"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 URL status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}

	// 合法清单保存后 GET 可见
	rec = adminRequest(t, handler, token, http.MethodPost, "/api/v2/admin/nodes",
		`{"nodes":[{"name":"us1","url":"https://us1.miawa.cn/"},{"name":"空名","url":""},{"name":"us1","url":"https://dup.example"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("合法保存 status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = adminGet(handler, "/api/v2/admin/nodes", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var env struct {
		Data struct {
			Nodes []config.NodeRoute `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	// NormalizeConfig：空项剔除、尾部斜杠剥除、重名去重
	if len(env.Data.Nodes) != 1 || env.Data.Nodes[0].Name != "us1" || env.Data.Nodes[0].URL != "https://us1.miawa.cn" {
		t.Fatalf("清洗后 nodes = %+v", env.Data.Nodes)
	}
}

func TestAdminConfigMasksNodeKeys(t *testing.T) {
	_, handler := setupNodesState(t, enableAdmin)
	token := mustAdminToken(t)

	rec := adminGet(handler, "/api/v2/admin/config", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	for _, key := range []string{"node_api_key", "parent_node_key"} {
		if v, _ := env.Data[key].(string); v != "" {
			t.Fatalf("GET 响应 %s 应为空掩码, got %q", key, v)
		}
	}
}
