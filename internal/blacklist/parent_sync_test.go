package blacklist

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
)

// SyncFromParent 端到端（httptest 假父节点）：携带 X-Node-Key 拉取信封包裹的
// 黑名单 → 本地 source=parent 对账落库。
func TestSyncFromParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "storage")
	if err := db.InitDB(dir, &config.Config{}); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			_ = db.DB.Close()
			db.DB = nil
		}
	})

	var gotKey string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Node-Key")
		if gotKey != "shared-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"ip": "198.51.100.8", "reason": "主服封禁", "source": "local", "ban_type": "traffic", "created_at": "2026-09-19 12:00:00"},
			},
		})
	}))
	t.Cleanup(fake.Close)

	if err := SyncFromParent(fake.URL, "shared-key", false); err != nil {
		t.Fatalf("SyncFromParent() error = %v", err)
	}
	banned, _, err := db.GetIPBlacklistInfo("198.51.100.8")
	if err != nil || !banned {
		t.Fatalf("父节点封禁应已落库: banned=%v err=%v", banned, err)
	}

	// 错误密钥：父节点拒绝，本地保持不变
	if err := SyncFromParent(fake.URL, "wrong", false); err == nil {
		t.Fatal("错误密钥应报错")
	}
}
