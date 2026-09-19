package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lemwood_mirror/internal/config"
)

func writeVersionIndex(t *testing.T, basePath string, launcher string, version string, isLatest bool) string {
	t.Helper()

	versionDir := filepath.Join(basePath, launcher, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	infoPath := filepath.Join(versionDir, "index.json")
	content, err := json.Marshal(map[string]any{
		"tag_name":  version,
		"is_latest": isLatest,
		"assets":    []any{},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if err := os.WriteFile(infoPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return infoPath
}

func TestTrimLauncherVersionsKeepsNewestThree(t *testing.T) {
	basePath := t.TempDir()
	cfg := &config.Config{}
	state := NewState(basePath, basePath, cfg)

	versions := []string{"v1.0.0", "v2.0.0", "v3.0.0", "v4.0.0", "v5.0.0"}
	for _, version := range versions {
		writeVersionIndex(t, basePath, "launcher", version, version == "v5.0.0")
	}

	if err := state.InitFromDisk(); err != nil {
		t.Fatalf("InitFromDisk() error = %v", err)
	}

	if err := state.TrimLauncherVersions("launcher", config.NormalizeMaxVersions(0), nil); err != nil {
		t.Fatalf("TrimLauncherVersions() error = %v", err)
	}

	wantPresent := []string{"v5.0.0", "v4.0.0", "v3.0.0"}
	for _, version := range wantPresent {
		if _, err := os.Stat(filepath.Join(basePath, "launcher", version)); err != nil {
			t.Fatalf("expected version %s to exist, stat error = %v", version, err)
		}
	}

	wantRemoved := []string{"v2.0.0", "v1.0.0"}
	for _, version := range wantRemoved {
		if _, err := os.Stat(filepath.Join(basePath, "launcher", version)); !os.IsNotExist(err) {
			t.Fatalf("expected version %s to be removed, stat error = %v", version, err)
		}
	}

	if got := state.GetLatestVersion("launcher"); got != "v5.0.0" {
		t.Fatalf("GetLatestVersion() = %s, want %s", got, "v5.0.0")
	}

	if got := len(state.index["launcher"]); got != 3 {
		t.Fatalf("len(index[launcher]) = %d, want 3", got)
	}

	if _, exists := state.infoCache[filepath.Join(basePath, "launcher", "v1.0.0", "index.json")]; exists {
		t.Fatal("expected removed version info cache to be deleted")
	}
}

func TestTrimLauncherVersionsNoopWhenWithinLimit(t *testing.T) {
	basePath := t.TempDir()
	cfg := &config.Config{}
	state := NewState(basePath, basePath, cfg)

	writeVersionIndex(t, basePath, "launcher", "v1.0.0", false)
	writeVersionIndex(t, basePath, "launcher", "v2.0.0", true)

	if err := state.InitFromDisk(); err != nil {
		t.Fatalf("InitFromDisk() error = %v", err)
	}

	if err := state.TrimLauncherVersions("launcher", 3, nil); err != nil {
		t.Fatalf("TrimLauncherVersions() error = %v", err)
	}

	if got := len(state.index["launcher"]); got != 2 {
		t.Fatalf("len(index[launcher]) = %d, want 2", got)
	}

	for _, version := range []string{"v1.0.0", "v2.0.0"} {
		if _, err := os.Stat(filepath.Join(basePath, "launcher", version)); err != nil {
			t.Fatalf("expected version %s to remain, stat error = %v", version, err)
		}
	}
}

// 复现 zl2/2.5.2 事故：上游删除 release 后本地目录成为孤儿，按版本号占着保留名额，
// 使扫描窗口内的版本（重新进入发布序列的旧版本）每轮被下载后立刻清理。
// 裁剪必须优先淘汰孤儿，保住扫描窗口内的版本。
func TestTrimLauncherVersionsEvictsOrphansFirst(t *testing.T) {
	basePath := t.TempDir()
	cfg := &config.Config{}
	state := NewState(basePath, basePath, cfg)

	// 磁盘 4 个版本；v2.0.0 上游已删除（孤儿），扫描窗口为发布序 top3
	for _, version := range []string{"v1.5.0", "v2.0.0", "v2.5.0_hotfix", "v3.0.0"} {
		writeVersionIndex(t, basePath, "launcher", version, version == "v3.0.0")
	}
	if err := state.InitFromDisk(); err != nil {
		t.Fatalf("InitFromDisk() error = %v", err)
	}

	upstream := map[string]bool{"v3.0.0": true, "v2.5.0_hotfix": true, "v1.5.0": true}
	if err := state.TrimLauncherVersions("launcher", 3, upstream); err != nil {
		t.Fatalf("TrimLauncherVersions() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(basePath, "launcher", "v2.0.0")); !os.IsNotExist(err) {
		t.Fatalf("expected orphan v2.0.0 to be removed, stat error = %v", err)
	}

	for _, version := range []string{"v3.0.0", "v2.5.0_hotfix", "v1.5.0"} {
		if _, err := os.Stat(filepath.Join(basePath, "launcher", version)); err != nil {
			t.Fatalf("expected version %s to exist, stat error = %v", version, err)
		}
	}

	if got := len(state.index["launcher"]); got != 3 {
		t.Fatalf("len(index[launcher]) = %d, want 3", got)
	}
}
