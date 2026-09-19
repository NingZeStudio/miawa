package db

import (
	"os"
	"path/filepath"
	"testing"

	"lemwood_mirror/internal/config"
)

// 黑名单父节点同步的全量对账契约：
// - parent 集合内的 IP 以 source='parent' 落库；
// - 上一轮的 parent 行被清除（父节点删禁 → 子节点解禁）；
// - 子节点自身的封禁（source=local 等）不受对账影响。
func TestReplaceParentBlacklistReconcile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "storage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir error = %v", err)
	}
	if err := InitDB(dir, &config.Config{}); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			_ = DB.Close()
			DB = nil
		}
	})

	// 子节点自有封禁（不得被对账清除）
	if err := AddIPToBlacklistWithSource("9.9.9.9", "本机自动封禁", "local", "rate_limit"); err != nil {
		t.Fatalf("seed local ban error = %v", err)
	}

	// 第一轮：parent 下发 {1.1.1.1, 3.3.3.3}
	first := []map[string]string{
		{"ip": "1.1.1.1", "reason": "主服封禁", "ban_type": "traffic", "created_at": "2026-09-19 10:00:00"},
		{"ip": "3.3.3.3", "reason": "主服旧封禁", "ban_type": "manual", "created_at": "2026-09-19 10:01:00"},
	}
	if err := ReplaceParentBlacklist(first); err != nil {
		t.Fatalf("first reconcile error = %v", err)
	}
	assertBlacklisted(t, "1.1.1.1", true)
	assertBlacklisted(t, "3.3.3.3", true)

	// 第二轮：父节点解封 3.3.3.3，新增 2.2.2.2
	second := []map[string]string{
		{"ip": "1.1.1.1", "reason": "主服封禁", "ban_type": "traffic", "created_at": "2026-09-19 10:00:00"},
		{"ip": "2.2.2.2", "reason": "主服新封禁", "ban_type": "manual", "created_at": "2026-09-19 11:00:00"},
	}
	if err := ReplaceParentBlacklist(second); err != nil {
		t.Fatalf("second reconcile error = %v", err)
	}
	assertBlacklisted(t, "1.1.1.1", true)
	assertBlacklisted(t, "2.2.2.2", true)
	assertBlacklisted(t, "3.3.3.3", false)
	assertBlacklisted(t, "9.9.9.9", true)

	// source 语义：parent 行可辨识
	info, _, err := GetIPBlacklistInfo("1.1.1.1")
	if err != nil || !info {
		t.Fatalf("GetIPBlacklistInfo(1.1.1.1) = %v, %v", info, err)
	}
}

func assertBlacklisted(t *testing.T, ip string, want bool) {
	t.Helper()
	banned, _, err := GetIPBlacklistInfo(ip)
	if err != nil {
		t.Fatalf("GetIPBlacklistInfo(%s) error = %v", ip, err)
	}
	if banned != want {
		t.Fatalf("IP %s blacklisted = %v, want %v", ip, banned, want)
	}
}
