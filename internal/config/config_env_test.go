package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEffectiveGitHubTokenEnvOverride 环境变量优先于配置文件中的值。
func TestEffectiveGitHubTokenEnvOverride(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token-123")
	cfg := DefaultConfig()
	cfg.GitHubToken = "disk-token"
	if got := cfg.EffectiveGitHubToken(); got != "env-token-123" {
		t.Fatalf("EffectiveGitHubToken() = %q, want env token", got)
	}
	os.Unsetenv("GITHUB_TOKEN")
	if got := cfg.EffectiveGitHubToken(); got != "disk-token" {
		t.Fatalf("无环境变量时应回退配置值, got %q", got)
	}
}

// TestLoadConfigDoesNotPersistEnvGitHubToken 环境变量 GITHUB_TOKEN 绝不能
// 经 LoadConfig 的"自动补齐缺失配置项"回写持久化到 config.yaml（密钥落盘）。
func TestLoadConfigDoesNotPersistEnvGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "super-secret-env-token")

	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.GitHubToken = "disk-token"
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	// 运行时生效的 token 仍应是环境变量值
	if got := loaded.EffectiveGitHubToken(); got != "super-secret-env-token" {
		t.Fatalf("运行时 token 应取环境变量, got %q", got)
	}
	// 但磁盘文件必须保持原值，不被环境变量污染
	b, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("读取 config.yaml 失败: %v", err)
	}
	if strings.Contains(string(b), "super-secret-env-token") {
		t.Fatalf("环境变量 token 被持久化到 config.yaml（安全缺陷）")
	}
	if !strings.Contains(string(b), "disk-token") {
		t.Fatalf("config.yaml 应保留磁盘原值 disk-token")
	}
}
