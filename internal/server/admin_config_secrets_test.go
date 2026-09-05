package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lemwood_mirror/internal/auth"
	"lemwood_mirror/internal/config"
)

// TestAdminConfigSecretsMaskAndKeep 配置 API 的秘密字段契约：
// GET 一律掩码（不回显密钥明文）；POST 留空 = 保持原值（不得静默清空）。
func TestAdminConfigSecretsMaskAndKeep(t *testing.T) {
	hashed, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	cfg := &config.Config{
		StoragePath:      "download",
		PowEnabled:       true,
		AdminEnabled:     true,
		AdminUser:        "admin",
		AdminPassword:    hashed,
		GitHubToken:      "gh-token-secret",
		PowHMACSecret:    "pow-secret",
		MySQLPassword:    "mysql-pass",
		PostgresPassword: "pg-pass",
	}
	state, handler, _ := setupDownloadHandlerState(t, cfg, 0, "hello")

	// GET：秘密字段必须掩码
	rec := adminGet(handler, "/api/v2/admin/config", mustAdminToken(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	for _, key := range []string{"github_token", "pow_hmac_secret", "mysql_password", "postgres_password", "admin_password"} {
		if v, _ := env.Data[key].(string); v != "" {
			t.Fatalf("GET 响应 %s 应为空掩码, got %q", key, v)
		}
	}

	// POST：带上掩码后的空秘密字段（前端真实行为），其他字段正常修改
	payload := map[string]any{
		"github_token":      "",
		"pow_hmac_secret":   "",
		"mysql_password":    "",
		"postgres_password": "",
		"admin_password":    "",
		"server_port":       8091,
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v2/admin/config", bytes.NewReader(body))
	postReq.Header.Set("Authorization", mustAdminToken(t))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", postRec.Code, postRec.Body.String())
	}

	// 落盘的 config.yaml 必须保留原密钥，且应用了非秘密字段的修改
	b, err := os.ReadFile(filepath.Join(state.ProjectRoot, "config.yaml"))
	if err != nil {
		t.Fatalf("读取 config.yaml 失败: %v", err)
	}
	content := string(b)
	for _, secret := range []string{"gh-token-secret", "pow-secret", "mysql-pass", "pg-pass"} {
		if !strings.Contains(content, secret) {
			t.Fatalf("config.yaml 丢失密钥 %q（POST 留空被静默清空）", secret)
		}
	}
	if !strings.Contains(content, "server_port: 8091") {
		t.Fatalf("非秘密字段修改未生效: %s", content)
	}
}

// TestAdminConfigPasswordChange POST 提供新密码时按 bcrypt 哈希生效。
func TestAdminConfigPasswordChange(t *testing.T) {
	hashed, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	cfg := &config.Config{
		StoragePath:   "download",
		PowEnabled:    true,
		AdminEnabled:  true,
		AdminUser:     "admin",
		AdminPassword: hashed,
	}
	state, handler, _ := setupDownloadHandlerState(t, cfg, 0, "hello")

	body, _ := json.Marshal(map[string]any{"admin_password": "new-password-xyz"})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v2/admin/config", bytes.NewReader(body))
	postReq.Header.Set("Authorization", mustAdminToken(t))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", postRec.Code, postRec.Body.String())
	}

	stored := state.Conf().AdminPassword
	if !auth.CheckPasswordHash("new-password-xyz", stored) {
		t.Fatalf("新密码未生效（存储值 %q）", stored)
	}
	if !strings.HasPrefix(stored, "$2") {
		t.Fatalf("存储值应为 bcrypt 哈希而非明文: %q", stored)
	}
}

func mustAdminToken(t *testing.T) string {
	t.Helper()
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	return token
}
